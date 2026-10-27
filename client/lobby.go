package client

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/v1rushb/neoncoup/audio"
	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/protocol"
)

type chatLine struct {
	name   string
	text   string
	system bool
	bot    bool
	seat   int
}

type lobbyScreen struct {
	chat     []chatLine
	input    lineInput
	chatting bool
}

func newLobbyScreen() lobbyScreen {
	return lobbyScreen{input: newLineInput("say: ", protocol.MaxChatLen)}
}

func (l *lobbyScreen) onChat(a *App, env protocol.Envelope) {
	l.chat = append(l.chat, decodeChat(env))
	if len(l.chat) > 200 {
		l.chat = l.chat[len(l.chat)-200:]
	}
}

func decodeChat(env protocol.Envelope) chatLine {
	if env.Type == protocol.TEmote {
		var e protocol.Emote
		_ = env.Into(&e)
		text := ""
		if e.ID >= 0 && e.ID < len(protocol.Emotes) {
			text = protocol.Emotes[e.ID]
		}
		return chatLine{name: e.Name, text: text, seat: e.Seat}
	}
	var c protocol.Chat
	_ = env.Into(&c)
	return chatLine{name: c.Name, text: c.Text, system: c.System, bot: c.Bot, seat: c.Seat}
}

func (l *lobbyScreen) key(a *App, msg tea.KeyPressMsg) tea.Cmd {
	if l.chatting {
		switch l.input.handle(msg) {
		case "enter":
			if t := strings.TrimSpace(l.input.Value()); t != "" && a.sess != nil {
				a.sess.Send(protocol.TChat, protocol.Chat{Text: t})
			}
			l.input.Set("")
			l.chatting = false
		case "esc":
			l.chatting = false
		}
		return nil
	}
	if a.sess == nil {
		return nil
	}
	host := a.welcome.Host
	st := a.lobbyMsg.Settings
	changed := false
	switch msg.String() {
	case "esc", "q":
		a.quitConfirm = true
	case "tab", "/":
		l.chatting = true
	case "enter":
		if host {
			a.sess.Send(protocol.TStart, nil)
		} else {
			a.sess.Send(protocol.TReady, protocol.Ready{Ready: !l.meReady(a)})
		}
		a.sound.SFX(audio.SFXSelect)
	case "r":
		a.sess.Send(protocol.TReady, protocol.Ready{Ready: !l.meReady(a)})
	case "+", "=":
		st.MaxPlayers, changed = min(6, st.MaxPlayers+1), host
	case "-":
		st.MaxPlayers, changed = max(2, st.MaxPlayers-1), host
	case "f":
		st.FillBots, changed = !st.FillBots, host
	case "d":
		st.BotLevel, changed = cycle(levels, st.BotLevel, 1), host
	case "w":
		st.WindowMS, changed = cycle(windowOptions, st.WindowMS, 1), host
	case "v":
		st.Speed, changed = cycle(protocol.Speeds, st.Speed, 1), host
	case "t":
		st.TurnMS, changed = cycle(turnOptions, st.TurnMS, 1), host
	case "g":
		st.Magistrate, changed = !st.Magistrate, host
	}
	if changed {
		a.sess.Send(protocol.TSettings, st)
		a.sound.SFX(audio.SFXSelect)
	}
	return nil
}

func (l *lobbyScreen) meReady(a *App) bool {
	for _, m := range a.lobbyMsg.Members {
		if m.ID == a.welcome.You {
			return m.Ready
		}
	}
	return false
}

func (l *lobbyScreen) view(a *App) string {
	t := a.t
	lm := a.lobbyMsg
	room := lm.Room
	if room == "" {
		room = a.welcome.Room
	}
	code := lipgloss.NewStyle().Foreground(theme.Bg).Background(theme.Acid).Bold(true).Padding(0, 2).Render(room)
	var header []string
	header = append(header, t.Title.Render("LOBBY")+"   "+code)
	if a.conn != nil && a.conn.Host != nil {
		for _, line := range a.conn.Host.Lines {
			header = append(header, t.Muted.Render(line))
		}
	} else if a.conn != nil && a.conn.Via != "" {
		header = append(header, t.Muted.Render("connected via "+a.conn.Via))
	}

	// Roster.
	var rows []string
	players, specs := 0, 0
	for _, m := range lm.Members {
		if m.Spectator {
			specs++
			continue
		}
		players++
		status := t.Warn.Render("NOT READY")
		if m.Ready || m.Host {
			status = t.Gain.Render("READY")
		}
		if !m.Connected {
			status = t.Loss.Render("OFFLINE " + a.spinner())
		}
		tags := ""
		if m.Host {
			tags += t.Hot.Render(" [HOST]")
		}
		if m.ID == a.welcome.You {
			tags += t.Accent.Render(" (you)")
		}
		rows = append(rows, fmt.Sprintf("%s %-18s %s%s", t.G("●", "*"), m.Name, status, tags))
	}
	bots := 0
	if lm.Settings.FillBots {
		bots = max(0, lm.Settings.MaxPlayers-players)
	}
	for i := range bots {
		rows = append(rows, t.Muted.Render(fmt.Sprintf("%s %-18s %s", t.G("○", "o"), fmt.Sprintf("bot #%d", i+1), strings.ToUpper(levelTitle(lm.Settings.BotLevel)))))
	}
	for i := players + bots; i < lm.Settings.MaxPlayers; i++ {
		rows = append(rows, t.Faint.Render(t.G("○", "o")+" (open seat)"))
	}
	if specs > 0 {
		rows = append(rows, "", t.Muted.Render(fmt.Sprintf("%d spectator(s) watching", specs)))
	}
	roster := t.Box.Width(46).Render(t.Accent.Render("RUNNERS") + "\n\n" + strings.Join(rows, "\n"))

	st := lm.Settings
	turn := "OFF"
	if st.TurnMS > 0 {
		turn = fmt.Sprintf("%ds", st.TurnMS/1000)
	}
	settings := []string{
		fmt.Sprintf("seats        %d", st.MaxPlayers),
		fmt.Sprintf("bot fill     %s (%s)", onOff(st.FillBots), st.BotLevel),
		fmt.Sprintf("speed        %s", speedTitle(st.Speed)),
		fmt.Sprintf("window       %s", windowLabel(st.WindowMS, st.Speed)),
		fmt.Sprintf("turn timer   %s", turn),
		fmt.Sprintf("magistrate   %s", onOff(st.Magistrate)),
		fmt.Sprintf("seat hold    %ds", st.ReconnectS),
	}
	keys := "enter ready/unready · tab chat · q leave"
	if a.welcome.Host {
		keys = "enter START · +/- seats · f bot fill · d difficulty · v speed\nw window · t turn timer · g magistrate · tab chat · q leave"
	}
	setBox := t.Box.Width(40).Render(t.Accent.Render("TABLE RULES") + "\n\n" + t.Text.Render(strings.Join(settings, "\n")))

	// Chat.
	var chat []string
	for _, c := range lastN(l.chat, 8) {
		chat = append(chat, renderChat(t, c, 70))
	}
	in := l.input.view(t, 70, l.chatting)
	if !l.chatting {
		in = t.Faint.Render("tab to chat")
	}
	chatBox := t.Box.Width(88).Height(11).Render(strings.Join(chat, "\n") + "\n" + in)

	body := lipgloss.JoinVertical(lipgloss.Left,
		strings.Join(header, "\n"), "",
		lipgloss.JoinHorizontal(lipgloss.Top, roster, "  ", setBox), "",
		chatBox, "", t.Faint.Render(t.A(keys)))
	return lipgloss.Place(a.w, a.h, lipgloss.Center, lipgloss.Center, t.A(body))
}

func renderChat(t *theme.Theme, c chatLine, width int) string {
	if c.system {
		return t.Faint.Render(trunc("// "+c.text, width))
	}
	nameSt := t.Accent
	if c.bot {
		nameSt = t.Hot
	}
	return nameSt.Render(c.name+":") + " " + t.Text.Render(trunc(c.text, max(1, width-len([]rune(c.name))-2)))
}

func lastN[T any](s []T, n int) []T {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func trunc(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 1 {
		return string(r[:w])
	}
	return string(r[:w-1]) + "…"
}
