//go:build windows
// +build windows

package tea

import (
	"context"
	"fmt"
	"io"

	"github.com/erikgeiser/coninput"
	localereader "github.com/mattn/go-localereader"
	"golang.org/x/sys/windows"
)

func readInputs(ctx context.Context, msgs chan<- Msg, input io.Reader) error {
	if coninReader, ok := input.(*conInputReader); ok {
		return readConInputs(ctx, msgs, coninReader.conin)
	}

	return readAnsiInputs(ctx, msgs, localereader.NewReader(input))
}

func readConInputs(ctx context.Context, msgsch chan<- Msg, con windows.Handle) error {
	var ps coninput.ButtonState // keep track of previous mouse state
	for {
		events, err := coninput.ReadNConsoleInputs(con, 16)
		if err != nil {
			return fmt.Errorf("read coninput events: %w", err)
		}

		for _, event := range events {
			var msgs []Msg
			switch e := event.Unwrap().(type) {
			case coninput.KeyEventRecord:
				if !e.KeyDown || e.VirtualKeyCode == coninput.VK_SHIFT {
					continue
				}

				for i := 0; i < int(e.RepeatCount); i++ {
					msgs = append(msgs, KeyMsg{
						Type:  keyType(e),
						Runes: []rune{e.Char},
						Alt:   e.ControlKeyState.Contains(coninput.LEFT_ALT_PRESSED | coninput.RIGHT_ALT_PRESSED),
					})
				}
			case coninput.WindowBufferSizeEventRecord:
				msgs = append(msgs, WindowSizeMsg{
					Width:  int(e.Size.X),
					Height: int(e.Size.Y),
				})
			case coninput.MouseEventRecord:
				event := mouseEvent(ps, e)
				msgs = append(msgs, event)
				ps = e.ButtonState
			case coninput.FocusEventRecord, coninput.MenuEventRecord:
				// ignore
			default: // unknown event
				continue
			}

			// Send all messages to the channel
			for _, msg := range msgs {
				select {
				case msgsch <- msg:
				case <-ctx.Done():
					err := ctx.Err()
					if err != nil {
						return fmt.Errorf("coninput context error: %w", err)
					}
					return err
				}
			}
		}
	}
}

func mouseEventButton(p, s coninput.ButtonState) (button MouseButton, action MouseAction) {
	btn := p ^ s
	action = MouseActionPress
	if btn&s == 0 {
		action = MouseActionRelease
	}

	switch btn {
	case coninput.FROM_LEFT_1ST_BUTTON_PRESSED: // left button
		button = MouseButtonLeft
	case coninput.RIGHTMOST_BUTTON_PRESSED: // right button
		button = MouseButtonRight
	case coninput.FROM_LEFT_2ND_BUTTON_PRESSED: // middle button
		button = MouseButtonMiddle
	case coninput.FROM_LEFT_3RD_BUTTON_PRESSED: // unknown (possibly mouse backward)
		button = MouseButtonBackward
	case coninput.FROM_LEFT_4TH_BUTTON_PRESSED: // unknown (possibly mouse forward)
		button = MouseButtonForward
	}

	return button, action
}

func mouseEvent(p coninput.ButtonState, e coninput.MouseEventRecord) MouseMsg {
	ev := MouseMsg{
		X:     int(e.MousePositon.X),
		Y:     int(e.MousePositon.Y),
		Alt:   e.ControlKeyState.Contains(coninput.LEFT_ALT_PRESSED | coninput.RIGHT_ALT_PRESSED),
		Ctrl:  e.ControlKeyState.Contains(coninput.LEFT_CTRL_PRESSED | coninput.RIGHT_CTRL_PRESSED),
		Shift: e.ControlKeyState.Contains(coninput.SHIFT_PRESSED),
	}
	switch e.EventFlags {
	case coninput.CLICK, coninput.DOUBLE_CLICK:
		ev.Button, ev.Action = mouseEventButton(p, e.ButtonState)
		if ev.Action == MouseActionRelease {
			ev.Type = MouseRelease
		}
		switch ev.Button {
		case MouseButtonLeft:
			ev.Type = MouseLeft
		case MouseButtonMiddle:
			ev.Type = MouseMiddle
		case MouseButtonRight:
			ev.Type = MouseRight
		case MouseButtonBackward:
			ev.Type = MouseBackward
		case MouseButtonForward:
			ev.Type = MouseForward
		}
	case coninput.MOUSE_WHEELED:
		if e.WheelDirection > 0 {
			ev.Button = MouseButtonWheelUp
			ev.Type = MouseWheelUp
		} else {
			ev.Button = MouseButtonWheelDown
			ev.Type = MouseWheelDown
		}
	case coninput.MOUSE_HWHEELED:
		if e.WheelDirection > 0 {
			ev.Button = MouseButtonWheelRight
			ev.Type = MouseWheelRight
		} else {
			ev.Button = MouseButtonWheelLeft
			ev.Type = MouseWheelLeft
		}
	case coninput.MOUSE_MOVED:
		ev.Button, _ = mouseEventButton(0, e.ButtonState)
		ev.Action = MouseActionMotion
		ev.Type = MouseMotion
	}

	return ev
}

func keyType(e coninput.KeyEventRecord) KeyType {
	code := e.VirtualKeyCode

	switch code {
	case coninput.VK_RETURN:
		return KeyEnter
	case coninput.VK_BACK:
		return KeyBackspace
	case coninput.VK_TAB:
		return KeyTab
	case coninput.VK_SPACE:
		return KeyRunes // this could be KeySpace but on unix space also produces KeyRunes
	case coninput.VK_ESCAPE:
		return KeyEscape
	case coninput.VK_UP:
		return KeyUp
	case coninput.VK_DOWN:
		return KeyDown
	case coninput.VK_RIGHT:
		return KeyRight
	case coninput.VK_LEFT:
		return KeyLeft
	case coninput.VK_HOME:
		return KeyHome
	case coninput.VK_END:
		return KeyEnd
	case coninput.VK_PRIOR:
		return KeyPgUp
	case coninput.VK_NEXT:
		return KeyPgDown
	case coninput.VK_DELETE:
		return KeyDelete
	default:
		if e.ControlKeyState&(coninput.LEFT_CTRL_PRESSED|coninput.RIGHT_CTRL_PRESSED) == 0 {
			return KeyRunes
		}

		switch e.Char {
		case '@':
			return KeyCtrlAt
		case '\x01':
			return KeyCtrlA
		case '\x02':
			return KeyCtrlB
		case '\x03':
			return KeyCtrlC
		case '\x04':
			return KeyCtrlD
		case '\x05':
			return KeyCtrlE
		case '\x06':
			return KeyCtrlF
		case '\a':
			return KeyCtrlG
		case '\b':
			return KeyCtrlH
		case '\t':
			return KeyCtrlI
		case '\n':
			return KeyCtrlJ
		case '\v':
			return KeyCtrlK
		case '\f':
			return KeyCtrlL
		case '\r':
			return KeyCtrlM
		case '\x0e':
			return KeyCtrlN
		case '\x0f':
			return KeyCtrlO
		case '\x10':
			return KeyCtrlP
		case '\x11':
			return KeyCtrlQ
		case '\x12':
			return KeyCtrlR
		case '\x13':
			return KeyCtrlS
		case '\x14':
			return KeyCtrlT
		case '\x15':
			return KeyCtrlU
		case '\x16':
			return KeyCtrlV
		case '\x17':
			return KeyCtrlW
		case '\x18':
			return KeyCtrlX
		case '\x19':
			return KeyCtrlY
		case '\x1a':
			return KeyCtrlZ
		case '\x1b':
			return KeyCtrlCloseBracket
		case '\x1c':
			return KeyCtrlBackslash
		case '\x1f':
			return KeyCtrlUnderscore
		}

		switch code {
		case coninput.VK_OEM_4:
			return KeyCtrlOpenBracket
		}

		return KeyRunes
	}
}
