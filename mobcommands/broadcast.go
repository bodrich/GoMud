package mobcommands

import (
	"fmt"

	"github.com/volte6/gomud/events"
	"github.com/volte6/gomud/mobs"
	"github.com/volte6/gomud/rooms"
	"github.com/volte6/gomud/term"
)

// Global chat room
func Broadcast(rest string, mob *mobs.Mob, room *rooms.Room) (bool, error) {

	events.AddToQueue(events.Broadcast{
		Text: fmt.Sprintf(`<ansi fg="black-bold">(broadcast)</ansi> <ansi fg="mobname">%s</ansi>: <ansi fg="yellow">%s</ansi>%s`, mob.Character.Name, rest, term.CRLFStr),
	})

	return true, nil
}
