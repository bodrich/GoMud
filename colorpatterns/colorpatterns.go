package colorpatterns

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// TODO: Load patterns from a config file.
type ColorizeStyle uint8

var (
	Words   ColorizeStyle = 0
	Once    ColorizeStyle = 1
	Stretch ColorizeStyle = 2

	colorsCompiled bool = false

	numericPatterns = map[string][]int{
		`blackandwhite`: {247, 231},
		`blue`:          {17, 18, 19, 20, 21, 27, 69, 117, 195},
		`brown`:         {58, 94, 94, 130, 130, 130, 178, 178, 179},
		`coupon`:        {147, 231},
		`cyan`:          {27, 33, 39, 45, 51, 87, 123, 159, 195},
		`flame`:         {124, 196, 202, 208, 214, 220, 226, 228, 230},
		`glowing`:       {184, 226, 227, 228, 229, 230, 231, 230, 229, 228, 227, 226, 184, 142, 100, 58},
		`gold`:          {172, 214, 214, 220, 220, 220, 226, 226},
		`gray`:          {0, 234, 237, 239, 242, 245, 248, 252, 15},
		`green`:         {22, 28, 34, 40, 46, 83, 120, 157, 194},
		`orange`:        {58, 94, 130, 166, 202, 208, 214, 216, 223},
		`peppermint`:    {196, 231},
		`pink`:          {225, 219, 213, 207, 201, 164, 127},
		`purple`:        {53, 54, 55, 56, 57, 99, 105, 147, 189},
		`rainbow`:       {196, 214, 226, 118, 51, 21, 93},
		`red`:           {52, 88, 124, 160, 196, 197, 204, 210, 217},
		`rust`:          {94, 130, 172, 214},
		`swamp`:         {58, 64, 64, 70, 70, 70, 36, 36, 79},
		`turquoise`:     {23, 29, 36, 42, 49, 86, 122, 158, 194},
		`vommit`:        {34, 112, 202, 214, 223},
		`zombie`:        {77, 77, 113, 72, 65, 78},
	}

	// Short tags
	ShortTagPatterns = map[string][]string{}
)

func GetColorPatternNames() []string {

	if !colorsCompiled {
		CompileColorPatterns()
	}

	ret := []string{}

	for name := range numericPatterns {
		ret = append(ret, name)
	}

	sort.Slice(ret, func(i, j int) bool { return ret[i] < ret[j] })

	return ret
}

func ApplyColorPattern(input string, pattern string, method ...ColorizeStyle) string {
	if pattern == `` {
		return input
	}
	patternValues, ok := numericPatterns[pattern]
	if !ok {
		return input
	}

	return ApplyColors(input, patternValues, method...)
}

func ApplyColors(input string, patternValues []int, method ...ColorizeStyle) string {

	patternValueLength := len(patternValues)

	newString := strings.Builder{}

	patternDir := 1
	patternPosition := 0
	inTagPlaceholder := false

	//
	// Tokenize existing ansi tags to avoid colorizing them
	//
	// Regular expression to match <ansi ...>...</ansi> tags
	re := regexp.MustCompile(`<ansi[^>]*>.*?</ansi>`)
	// Counter to keep track of the unique numbers
	counter := 0
	preExistingTags := map[string]string{}
	// Function to replace each match with a unique number
	input = re.ReplaceAllStringFunc(input, func(match string) string {
		counter++
		tag := `:` + strconv.Itoa(counter)
		preExistingTags[tag] = match
		return tag
	})
	//
	// End tokenization
	//

	if len(method) == 0 {
		// Color change on a per character basis (not spaces), reverses at the end
		for _, runeChar := range input {

			// Handle placeholder tags that look like :123
			if inTagPlaceholder {
				if runeChar != 32 {
					newString.WriteString(string(runeChar))
					continue
				}
				inTagPlaceholder = false
			} else {
				if runeChar == ':' {
					inTagPlaceholder = true
					newString.WriteString(string(runeChar))
					continue
				}
			}

			newString.WriteString(fmt.Sprintf(`<ansi fg="%d">%s</ansi>`, patternValues[patternPosition], string(runeChar)))
			if runeChar != 32 { // space
				if patternPosition == patternValueLength-1 {
					patternDir = -1
				} else if patternPosition == 0 {
					patternDir = 1
				}
				patternPosition += patternDir // advance the color token position
			}
		}
	} else if method[0] == Words {
		// Color change on a per word basis

		newString.WriteString(`<ansi>`)
		for i, runeChar := range input {

			// Handle placeholder tags that look like :123
			if inTagPlaceholder {
				if runeChar != 32 {
					newString.WriteString(string(runeChar))
					continue
				}
				inTagPlaceholder = false
			} else {
				if runeChar == ':' {
					inTagPlaceholder = true
					newString.WriteString(string(runeChar))
					continue
				}
			}
			// End handling placeholder tags

			if i == 0 || runeChar == 32 { // space
				newString.WriteString(fmt.Sprintf(`</ansi><ansi fg="%d">`, patternValues[patternPosition%patternValueLength]))
				patternPosition++ // advance the color token position
			}
			newString.WriteRune(runeChar) // Write whatever the next character is
		}
		newString.WriteString(`</ansi>`)
	} else if method[0] == Once {
		// Color stops changing and stays on the final color
		newString.WriteString(`<ansi>`)
		for _, runeChar := range input {

			// Handle placeholder tags that look like :123
			if inTagPlaceholder {
				if runeChar != 32 {
					newString.WriteString(string(runeChar))
					continue
				}
				inTagPlaceholder = false
			} else {
				if runeChar == ':' {
					inTagPlaceholder = true
					newString.WriteString(string(runeChar))
					continue
				}
			}
			// End handling placeholder tags

			newString.WriteString(fmt.Sprintf(`<ansi fg="%d">%s</ansi>`, patternValues[patternPosition], string(runeChar)))
			if patternPosition < patternValueLength-1 && runeChar != 32 { // space
				patternPosition += 1 // advance the color token position
			}
		}
		newString.WriteString(`</ansi>`)
	} else if method[0] == Stretch {
		// Spread the whole pattern to fit the string
		subCounter := 0
		stretchAmount := int(math.Floor(float64(utf8.RuneCountInString(input)) / float64(len(patternValues))))
		if stretchAmount < 1 {
			stretchAmount = 1
		}
		newString.WriteString(`<ansi>`)
		for _, runeChar := range input {

			// Handle placeholder tags that look like :123
			if inTagPlaceholder {
				if runeChar != 32 {
					newString.WriteString(string(runeChar))
					continue
				}
				inTagPlaceholder = false
			} else {
				if runeChar == ':' {
					inTagPlaceholder = true
					newString.WriteString(string(runeChar))
					continue
				}
			}
			// End handling placeholder tags

			newString.WriteString(fmt.Sprintf(`<ansi fg="%d">%s</ansi>`, patternValues[patternPosition], string(runeChar)))
			subCounter++
			if patternPosition < patternValueLength-1 && runeChar != 32 { // space
				if subCounter%stretchAmount == 0 {
					patternPosition += 1 // advance the color token position
				}
			}
		}
		newString.WriteString(`</ansi>`)
	}

	finalString := newString.String()

	for tmp, replacement := range preExistingTags {
		finalString = strings.Replace(finalString, tmp, replacement, -1)
	}

	return finalString
}

func CompileColorPatterns() {

	if colorsCompiled {
		return
	}

	for name, numbers := range numericPatterns {
		cPatterns := []string{}

		for _, num := range numbers {
			cPatterns = append(cPatterns, fmt.Sprintf(`{%d}`, num))
		}
		ShortTagPatterns[name] = cPatterns
	}

	colorsCompiled = true
}

func IsValidPattern(pName string) bool {
	if _, ok := numericPatterns[pName]; ok {
		return true
	}
	return false
}

func init() {
	CompileColorPatterns()
}
