package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	CharColor     byte = '\x03'
	CharReset     byte = '\x0f'
	CharBold      byte = '\x02'
	CharItalic    byte = '\x1d'
	CharUnderline byte = '\x1f'
)

var re = regexp.MustCompile(fmt.Sprintf(`(?:(%[2]s)|(?:(%[1]s)(?:(\d{1,2})(?:,(\d{1,2}))?)?))?([^%[1]s%[2]s]*)`, CharToHex(CharColor), CharToHex(CharReset)))
var re2 = regexp.MustCompile(fmt.Sprintf(`(%[1]s|%[2]s|%[3]s)?([^%[1]s%[2]s%[3]s]*)`, CharToHex(CharBold), CharToHex(CharItalic), CharToHex(CharUnderline)))
var re3 = regexp.MustCompile(fmt.Sprintf(`(?m)^(?:%s\d{1,2})?\[\d{2}:\d{2}:\d{2}] (?:-> )?\*.+?\*`, CharToHex(CharColor)))

func CharToHex(c byte) string {
	return fmt.Sprintf("\\x%02x", c)
}

type ColorNumber int8
type ColorState uint8
type Index int8

const (
	ColorNone   ColorNumber = -1
	ColorTransp ColorNumber = 99
)

const (
	_ ColorState = iota
	StateCancel
	StateNew
	StateFg
	StateBg
	StateBoth
	StateSame
)

type ColorCode struct {
	fg ColorNumber
	bg ColorNumber
}

type FormatCode struct {
	id    Index
	char  byte
	text  string
	color ColorCode
}

type Tag struct {
	char  byte
	color ColorCode
}

func (tag *Tag) ToString(close bool) {
	if tag.char == CharColor {
		name := `span`

		if !close {
			var class string

			if tag.color.fg != ColorNone && tag.color.fg != ColorTransp {
				class = fmt.Sprintf("fc%d", tag.color.fg)
			}

			if tag.color.bg != ColorNone && tag.color.bg != ColorTransp {
				if len(class) > 0 {
					class += " "
				}
				class += fmt.Sprintf("bc%d", tag.color.bg)
			}

			name += fmt.Sprintf(` class="%s"`, strings.TrimSpace(class))
		}

		conv.WriteByte('<')
		if close {
			conv.WriteByte('/')
		}
		conv.WriteString(name)
		conv.WriteByte('>')
	} else {
		var name byte

		if tag.char == CharBold {
			name = 'b'
		}
		if tag.char == CharItalic {
			name = 'i'
		}
		if tag.char == CharUnderline {
			name = 'u'
		}

		conv.WriteByte('[')
		if close {
			conv.WriteByte('/')
		}
		conv.WriteByte(name)
		conv.WriteByte(']')
	}
}

func (cc *ColorCode) GetState(other *ColorCode) ColorState {
	isFgNone, isBgNone := cc.fg == ColorNone, cc.bg == ColorNone
	isFg2None, isBg2None := other.fg == ColorNone, other.bg == ColorNone

	if (isFgNone && isBgNone) || (cc.fg == ColorTransp && cc.bg == ColorTransp) {
		return StateCancel
	}

	if isFg2None && isBg2None {
		return StateNew
	}

	isFgChanged := cc.fg != other.fg
	isBgChanged := cc.bg != other.bg && !isBgNone

	if isFgChanged {
		if isBgChanged || (isBgNone && isBg2None) {
			return StateBoth
		} else {
			return StateFg
		}
	} else {
		if isBgChanged {
			if isFgNone && isFg2None {
				return StateBoth
			}
			return StateBg
		} else {
			return StateSame
		}
	}
}

var codes []*FormatCode
var tags []*Tag
var conv *strings.Builder
var opened map[byte]bool

func GetCurrentColor() *ColorCode {
	r := ColorCode{fg: ColorNone, bg: ColorNone}

	for i := len(tags) - 1; i >= 0; i-- {
		tag := tags[i]
		if tag.char == CharColor {
			if r.fg == ColorNone && tag.color.fg != ColorNone {
				r.fg = tag.color.fg
			}
			if r.bg == ColorNone && tag.color.bg != ColorNone {
				r.bg = tag.color.bg
			}
			if r.fg != ColorNone && r.bg != ColorNone {
				return &r
			}
		} else {
			continue
		}
	}

	return &r
}

func AddTag(t *Tag) {
	tags = append(tags, t)
	t.ToString(false)
}

func AddFormatTag(c byte) {
	AddTag(&Tag{char: c})
}

func AddColorTag(fg ColorNumber, bg ColorNumber) {
	if fg != ColorNone && fg != ColorTransp || bg != ColorNone && bg != ColorTransp {
		AddTag(&Tag{char: CharColor, color: ColorCode{fg: fg, bg: bg}})
	}
}

func CloseTag(i Index) {
	if i >= 0 && i < Index(len(tags)) {
		tags[i].ToString(true)
		tags = append(tags[:i], tags[i+1:]...)
	}
}

func GetLastTagIndex(c byte) Index {
	for i := Index(len(tags)) - 1; i >= 0; i-- {
		if tags[i].char == c {
			return i
		}
	}

	return -1
}

func Reset(colors bool) {
	for i := Index(len(tags)) - 1; i >= 0; i-- {
		tag := tags[i]
		if tag.char != CharColor && colors {
			continue
		}
		tag.ToString(true)
	}

	tags = nil
}

func ConvertLine(s string) string {
	if conv == nil {
		conv = &strings.Builder{}
	}
	conv.Reset()

	matches := re.FindAllStringSubmatch(s, -1)
	codes = []*FormatCode{}

	for i, match := range matches {
		text := match[5]

		if len(match[2]) > 0 && match[2][0] == CharColor {
			fg, err := strconv.ParseInt(match[3], 10, 8)
			if err != nil {
				fg = int64(ColorNone)
			}
			bg, err := strconv.ParseInt(match[4], 10, 8)
			if err != nil {
				bg = int64(ColorNone)
			}
			if fg == 1 && ColorNumber(bg) == ColorNone && i == 0 {
				conv.WriteString(text)
			} else {
				code := FormatCode{
					id:   Index(i),
					char: CharColor,
					text: text,
					color: ColorCode{
						fg: ColorNumber(fg),
						bg: ColorNumber(bg)}}

				p := GetLastCodeIndex(CharColor)
				if p >= 0 {
					prev := codes[p]
					if len(prev.text) == 0 {
						if prev.color.fg != ColorNone {
							if code.color.bg == ColorNone && code.color.fg != ColorNone {
								code.color.bg = prev.color.bg
							}
							codes = append(codes[:p], codes[p+1:]...)
						}
					}
				}

				codes = append(codes, &code)
			}
		} else if len(match[1]) > 0 && match[1][0] == CharReset {
			code := FormatCode{
				id:   Index(i),
				char: match[1][0],
				text: text}
			codes = append(codes, &code)
		} else {
			conv.WriteString(text)
		}
	}

	last := len(codes) - 1
	if last >= 0 {
		if len(codes[last].text) == 0 {
			codes = codes[:last]
		}
	}

	if opened == nil {
		opened = map[byte]bool{}
	}
	for k := range opened {
		delete(opened, k)
	}

	for i, code := range codes {
		ConvertFormatCode(Index(i), code)
	}

	Reset(false)

	return conv.String()
}

func GetLastCodeIndex(c byte) Index {
	for i := Index(len(codes)) - 1; i >= 0; i-- {
		if codes[i].char == c {
			return i
		}
	}

	return -1
}

func ConvertFormatCode(i Index, c *FormatCode) {
	if c.char == CharColor {
		state := c.color.GetState(GetCurrentColor())

		// 1. закрытие тегов

		if state == StateCancel {
			Reset(true)
			state = c.color.GetState(GetCurrentColor())
		} else if state == StateFg || state == StateBg || state == StateBoth {
			ShouldCloseAllColorTags(i)
			state = c.color.GetState(GetCurrentColor())
		}

		// 2. открытие тегов

		isSpace := false
		if len(strings.TrimSpace(c.text)) == 0 {
			isSpace = true
		}

		if state == StateBoth || state == StateNew {
			fg := c.color.fg
			if isSpace {
				fg = ColorNone
			}
			AddColorTag(fg, c.color.bg)
		} else if state == StateFg {
			if !isSpace {
				AddColorTag(c.color.fg, ColorNone)
			}
		} else if state == StateBg {
			AddColorTag(ColorNone, c.color.bg)
		}
	} else if c.char == CharReset {
		Reset(false)
	}

	for _, match := range re2.FindAllStringSubmatch(c.text, -1) {
		text := match[2]

		if len(match[1]) > 0 {
			char := match[1][0]
			if opened[char] {
				CloseTag(GetLastTagIndex(char))
			} else {
				AddFormatTag(char)
			}
			opened[char] = !opened[char]
		}

		conv.WriteString(text)
	}
}

func ShouldCloseColorTag(t *Tag, start Index) bool {
	for i := start; i < Index(len(codes)); i++ {
		if t.char == CharColor {
			code := codes[i]

			if code.char == CharColor {
				state := code.color.GetState(&t.color)

				if state == StateCancel ||
					t.color.fg != ColorTransp && code.color.fg == ColorTransp ||
					t.color.bg != ColorTransp && code.color.bg == ColorTransp {
					return true
				}

				if state == StateFg || state == StateBg || state == StateSame {
					return false
				}
			}
		}
	}

	return true
}

func ShouldCloseAllColorTags(start Index) {
	for i := Index(len(tags)) - 1; i >= 0; i-- {
		tag := tags[i]
		if tag.char == CharColor && ShouldCloseColorTag(tag, start) {
			CloseTag(i)
		} else {
			break
		}
	}
}

func WriteLine(w io.Writer, s string) {
	if _, err := fmt.Fprintf(w, "%s\n", s); err != nil {
		log.Fatal(err)
	}
}

func ConvertFile(i string, o string, align bool, priv bool) {
	in, err := os.Open(i)
	if err != nil {
		log.Fatal(err)
	}
	defer in.Close()

	s := bufio.NewScanner(in)
	s.Split(bufio.ScanLines)

	out, err := os.Create(o)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	w := bufio.NewWriter(out)
	if align {
		WriteLine(w, strings.Repeat("—", 132))
	}

	for s.Scan() {
		if line := s.Text(); priv || !re3.MatchString(line) {
			WriteLine(w, ConvertLine(line))
		}
	}

	if err := s.Err(); err != nil {
		log.Fatal(err)
	}

	if err := w.Flush(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	log := flag.String("log", "input.log", "input log file name")
	output := flag.String("output", "output.log", "output log file name")
	align := flag.Bool("align", false, "append alignment line")
	priv := flag.Bool("priv", false, "include private messages")

	flag.Parse()

	ConvertFile(*log, *output, *align, *priv)
}
