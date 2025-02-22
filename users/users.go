package users

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"github.com/volte6/gomud/characters"
	"github.com/volte6/gomud/configs"
	"github.com/volte6/gomud/connections"
	"github.com/volte6/gomud/mobs"
	"github.com/volte6/gomud/util"

	//
	"gopkg.in/yaml.v2"
)

const minimumUsernameLength = 2
const maximumUsernameLength = 16
const minimumPasswordLength = 4
const maximumPasswordLength = 16

var (
	userManager *ActiveUsers = newUserManager()
)

type ActiveUsers struct {
	Users             map[int]*UserRecord                 // userId to UserRecord
	Usernames         map[string]int                      // username to userId
	Connections       map[connections.ConnectionId]int    // connectionId to userId
	UserConnections   map[int]connections.ConnectionId    // userId to connectionId
	ZombieConnections map[connections.ConnectionId]uint64 // connectionId to turn they became a zombie
}

type Online struct {
	UserId         string
	Username       string
	Permission     string
	CharacterName  string
	CharacterLevel int
	Zone           string
	RoomId         int
}

func newUserManager() *ActiveUsers {
	return &ActiveUsers{
		Users:             make(map[int]*UserRecord),
		Usernames:         make(map[string]int),
		Connections:       make(map[connections.ConnectionId]int),
		UserConnections:   make(map[int]connections.ConnectionId),
		ZombieConnections: make(map[connections.ConnectionId]uint64),
	}
}

func RemoveZombieUser(userId int) {

	if u := userManager.Users[userId]; u != nil {
		u.Character.SetAdjective(`zombie`, false)
	}
	connId := userManager.UserConnections[userId]
	delete(userManager.ZombieConnections, connId)
}

func RemoveZombieConnection(connectionId connections.ConnectionId) {

	delete(userManager.ZombieConnections, connectionId)
}

// Returns a slice of userId's
// These userId's are zombies that have reached expiration
func GetExpiredZombies(expirationTurn uint64) []int {

	expiredUsers := make([]int, 0)

	for connectionId, zombieTurn := range userManager.ZombieConnections {
		if zombieTurn < expirationTurn {
			expiredUsers = append(expiredUsers, userManager.Connections[connectionId])
		}
	}

	return expiredUsers
}

func GetConnectionIds(userIds []int) []connections.ConnectionId {

	connectionIds := make([]connections.ConnectionId, 0, len(userIds))
	for _, userId := range userIds {
		if user, ok := userManager.Users[userId]; ok {
			connectionIds = append(connectionIds, user.connectionId)
		}
	}

	return connectionIds
}

func GetAllActiveUsers() []*UserRecord {
	ret := []*UserRecord{}

	for _, userPtr := range userManager.Users {
		if !userPtr.isZombie {
			ret = append(ret, userPtr)
		}
	}

	return ret
}

func GetOnlineUserIds() []int {

	onlineList := make([]int, 0, len(userManager.Users))
	for _, user := range userManager.Users {
		onlineList = append(onlineList, user.UserId)
	}
	return onlineList
}

func GetOnlineList() []Online {

	onlineList := make([]Online, 0, len(userManager.Users))
	for _, user := range userManager.Users {
		onlineList = append(onlineList, Online{
			UserId:         strconv.Itoa(int(user.UserId)),
			Username:       user.Username,
			Permission:     user.Permission,
			CharacterName:  user.Character.Name,
			CharacterLevel: user.Character.Level,
			Zone:           user.Character.Zone,
			RoomId:         user.Character.RoomId,
		})
	}

	return onlineList
}

func GetByCharacterName(name string) *UserRecord {

	var closeMatch *UserRecord = nil

	name = strings.ToLower(name)
	for _, user := range userManager.Users {
		testName := strings.ToLower(user.Character.Name)
		if testName == name {
			return user
		}
		if strings.HasPrefix(testName, name) {
			closeMatch = user
		}
	}

	return closeMatch
}

func GetByUserId(userId int) *UserRecord {

	if user, ok := userManager.Users[userId]; ok {
		return user
	}

	return nil
}

func GetByConnectionId(connectionId connections.ConnectionId) *UserRecord {

	if userId, ok := userManager.Connections[connectionId]; ok {
		return userManager.Users[userId]
	}

	return nil
}

// First time creating a user.
func LoginUser(u *UserRecord, connectionId connections.ConnectionId) (*UserRecord, string, error) {

	slog.Info("LoginUser()", "username", u.Username, "connectionId", connectionId)

	u.Character.SetAdjective(`zombie`, false)

	if userId, ok := userManager.Usernames[u.Username]; ok {

		if otherConnId, ok := userManager.UserConnections[userId]; ok {

			if _, ok := userManager.ZombieConnections[otherConnId]; ok {

				slog.Info("LoginUser()", "Zombie", true)

				if zombieUser, ok := userManager.Users[u.UserId]; ok {
					u = zombieUser
				}

				// The user is a zombie.
				delete(userManager.ZombieConnections, otherConnId)

				u.connectionId = connectionId

				userManager.Users[u.UserId] = u
				userManager.Usernames[u.Username] = u.UserId
				userManager.Connections[u.connectionId] = u.UserId
				userManager.UserConnections[u.UserId] = u.connectionId

				for _, mobInstId := range u.Character.GetCharmIds() {
					if !mobs.MobInstanceExists(mobInstId) {
						u.Character.TrackCharmed(mobInstId, false)
					}
				}

				// Set their input round to current to track idle time fresh
				u.SetLastInputRound(util.GetRoundCount())

				return u, "Reconnecting...", nil
			}

		}

		return nil, "That user is already logged in.", errors.New("user is already logged in")
	}

	if len(u.AdminCommands) > 0 {
		u.Permission = PermissionMod
	}

	slog.Info("LoginUser()", "Zombie", false)

	// Set their input round to current to track idle time fresh
	u.SetLastInputRound(util.GetRoundCount())

	u.connectionId = connectionId

	userManager.Users[u.UserId] = u
	userManager.Usernames[u.Username] = u.UserId
	userManager.Connections[u.connectionId] = u.UserId
	userManager.UserConnections[u.UserId] = u.connectionId

	slog.Info("LOGIN", "userId", u.UserId)
	for _, mobInstId := range u.Character.GetCharmIds() {
		if !mobs.MobInstanceExists(mobInstId) {
			u.Character.TrackCharmed(mobInstId, false)
		}
	}

	return u, "", nil
}

func SetZombieUser(userId int) {

	if u, ok := userManager.Users[userId]; ok {

		u.Character.RemoveBuff(0)
		u.Character.SetAdjective(`zombie`, true)

		// Prevent guide mob dupes
		for _, miid := range u.Character.CharmedMobs {
			if m := mobs.GetInstance(miid); m != nil {
				if m.MobId == 38 {
					m.Character.Charmed.RoundsRemaining = 0
				}
			}
		}

		if _, ok := userManager.ZombieConnections[u.connectionId]; ok {
			return
		}

		userManager.ZombieConnections[u.connectionId] = util.GetTurnCount()
	}

}

func SaveAllUsers() {

	for _, u := range userManager.Users {
		if err := SaveUser(*u); err != nil {
			slog.Error("SaveAllUsers()", "error", err.Error())
		}
	}

}

func LogOutUserByConnectionId(connectionId connections.ConnectionId) error {

	u := GetByConnectionId(connectionId)

	if _, ok := userManager.Connections[connectionId]; ok {

		// Make sure the user data is saved to a file.
		if u != nil {
			u.Character.Validate()
			SaveUser(*u)
		}

		delete(userManager.Users, u.UserId)
		delete(userManager.Usernames, u.Username)
		delete(userManager.Connections, u.connectionId)
		delete(userManager.UserConnections, u.UserId)

		return nil
	}
	return errors.New("user not found for connection")
}

// First time creating a user.
func CreateUser(u *UserRecord) error {

	if err := util.ValidateName(u.Username); err != nil {
		return errors.New("that username is not allowed: " + err.Error())
	}

	if bannedPattern, ok := configs.GetConfig().IsBannedName(u.Username); ok {
		return errors.New(`that username matched the prohibited name pattern: "` + bannedPattern + `"`)
	}

	for _, name := range mobs.GetAllMobNames() {
		if strings.EqualFold(name, u.Username) {
			return errors.New("that username is in use")
		}
	}

	if Exists(u.Username) {
		return errors.New("that username is in use")
	}

	u.UserId = GetUniqueUserId()
	u.Permission = PermissionUser

	//if err := SaveUser(*u); err != nil {
	//return err
	//}

	userManager.Users[u.UserId] = u
	userManager.Usernames[u.Username] = u.UserId
	userManager.Connections[u.connectionId] = u.UserId
	userManager.UserConnections[u.UserId] = u.connectionId

	return nil
}

func LoadUser(username string) (*UserRecord, error) {
	if !Exists(strings.ToLower(username)) {
		return nil, errors.New("user already exists")
	}

	slog.Info("Loading user", "username", username)

	userFilePath := util.FilePath(string(configs.GetConfig().FolderUserData), `/`, strings.ToLower(username)+`.yaml`)

	userFileTxt, err := os.ReadFile(userFilePath)
	if err != nil {
		return nil, err
	}

	loadedUser := &UserRecord{}
	if err := yaml.Unmarshal([]byte(userFileTxt), loadedUser); err != nil {
		slog.Error("LoadUser", "error", err.Error())
	}

	rebuiltMemory := []int{}
	memoryString := string(util.Decompress(util.Decode(loadedUser.RoomMemoryBlob)))
	for _, rId := range strings.Split(memoryString, ",") {
		if rIdInt, err := strconv.Atoi(rId); err == nil {
			rebuiltMemory = append(rebuiltMemory, rIdInt)
		}
	}

	loadedUser.Character.SetRoomMemory(rebuiltMemory)
	loadedUser.RoomMemoryBlob = ``

	if loadedUser.Joined.IsZero() {
		loadedUser.Joined = time.Now()
	}

	if err := loadedUser.Character.Validate(true); err == nil {
		SaveUser(*loadedUser)
	}

	// Set their connection time to now
	loadedUser.connectionTime = time.Now()

	return loadedUser, nil
}

// Loads all user recvords and runs against a function.
// Stops searching if false is returned.
func SearchOfflineUsers(searchFunc func(u *UserRecord) bool) {

	basePath := util.FilePath(string(configs.GetConfig().FolderUserData))

	filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {

		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if len(path) > 10 && path[len(path)-10:] == `-alts.yaml` {
			return nil
		}

		var uRecord UserRecord

		fpathLower := path[len(path)-5:] // Only need to compare the last 5 characters
		if fpathLower == `.yaml` {

			bytes, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			err = yaml.Unmarshal(bytes, &uRecord)
			if err != nil {
				return err
			}

			// If this is an online user, skip it
			if _, ok := userManager.Usernames[uRecord.Username]; ok {
				return nil
			}

			if res := searchFunc(&uRecord); !res {
				return errors.New(`done searching`)
			}
		}
		return nil
	})

}

// searches for a character name and returns the user that owns it
// Slow and possibly memory intensive - use strategically
func CharacterNameSearch(nameToFind string) (foundUserId int, foundUserName string) {

	foundUserId = 0
	foundUserName = ``

	SearchOfflineUsers(func(u *UserRecord) bool {

		if strings.EqualFold(u.Character.Name, nameToFind) {
			foundUserId = u.UserId
			foundUserName = u.Username
			return false
		}

		// Not found? Search alts...

		for _, char := range characters.LoadAlts(u.Username) {
			if strings.EqualFold(char.Name, nameToFind) {
				foundUserId = u.UserId
				foundUserName = u.Username
				return false
			}
		}

		return true
	})

	return foundUserId, foundUserName
}

func SaveUser(u UserRecord) error {

	fileWritten := false
	tmpSaved := false
	tmpCopied := false
	completed := false

	defer func() {
		slog.Info("SaveUser()", "username", u.Username, "wrote-file", fileWritten, "tmp-file", tmpSaved, "tmp-copied", tmpCopied, "completed", completed)
	}()

	// Don't save if they haven't entered the real game world yet.
	//if u.Character.RoomId < 0 {
	//return errors.New("Has not started game.")
	//}

	if u.Character.RoomId >= 900 && u.Character.RoomId <= 999 {
		u.Character.RoomId = -1
	}

	memoryString := ``
	for _, rId := range u.Character.GetRoomMemory() {
		memoryString += strconv.Itoa(rId) + ","
	}
	memoryString = strings.TrimSuffix(memoryString, ",")

	u.RoomMemoryBlob = util.Encode(util.Compress([]byte(memoryString)))

	data, err := yaml.Marshal(&u)
	if err != nil {
		return err
	}

	carefulSave := configs.GetConfig().CarefulSaveFiles

	path := util.FilePath(string(configs.GetConfig().FolderUserData), `/`, strings.ToLower(u.Username)+`.yaml`)

	saveFilePath := path
	if carefulSave { // careful save first saves a {filename}.new file
		saveFilePath += `.new`
	}

	err = os.WriteFile(saveFilePath, data, 0777)
	if err != nil {
		return err
	}
	fileWritten = true
	if carefulSave {
		tmpSaved = true
	}

	if carefulSave {
		//
		// Once the file is written, rename it to remove the .new suffix and overwrite the old file
		//
		if err := os.Rename(saveFilePath, path); err != nil {
			return err
		}
		tmpCopied = true
	}

	completed = true

	return nil
}

func GetUniqueUserId() int {
	return UserCount() + 1
}

func Exists(name string) bool {
	_, err := os.Stat(util.FilePath(string(configs.GetConfig().FolderUserData), `/`, strings.ToLower(name)+`.yaml`))
	return !os.IsNotExist(err)
}

func UserCount() int {

	entries, err := os.ReadDir(util.FilePath(string(configs.GetConfig().FolderUserData)))
	if err != nil {
		panic(err)
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			count++
		}
	}
	return count
}
