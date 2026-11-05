package desktop

import (
	"regexp"
	"strings"
)

// The guards (8.10), the same for every model. Names are matched in
// English, German and Persian.

// denied are programs whose windows can't be picked: terminals and shells,
// system tools, password managers, and the sign-in and UAC screens. Jenab
// itself is denied by its process.
var denied = map[string]string{
	"windowsterminal.exe": "a terminal", "cmd.exe": "a terminal", "powershell.exe": "a terminal",
	"pwsh.exe": "a terminal", "conhost.exe": "a terminal", "openconsole.exe": "a terminal",
	"wsl.exe": "a terminal", "wslhost.exe": "a terminal", "bash.exe": "a terminal", "mintty.exe": "a terminal",
	"wt.exe": "a terminal", "alacritty.exe": "a terminal", "wezterm-gui.exe": "a terminal",
	"regedit.exe": "a system tool", "mmc.exe": "a system tool", "taskmgr.exe": "a system tool",
	"systemsettings.exe": "a system tool", "control.exe": "a system tool", "taskschd.exe": "a system tool",
	"1password.exe": "a password manager", "keepass.exe": "a password manager", "keepassxc.exe": "a password manager",
	"bitwarden.exe": "a password manager", "dashlane.exe": "a password manager", "enpass.exe": "a password manager",
	"nordpass.exe": "a password manager", "proton pass.exe": "a password manager", "lastpass.exe": "a password manager",
	"roboform.exe": "a password manager",
	"consent.exe":  "a sign-in screen", "logonui.exe": "a sign-in screen", "credentialuibroker.exe": "a sign-in screen",
	"lockapp.exe": "a sign-in screen", "credentialenrollmentmanager.exe": "a sign-in screen",
}

// pickable reports whether w can be offered in the pick card, and why not.
// self is Jenab's process id; class is the window's class name, which
// tells the Run box (a dialog of Explorer). A browser on a password
// manager's page is told by its title, which follows the open tab.
func pickable(w Window, self int, class string) (bool, string) {
	app := strings.ToLower(w.App)
	switch {
	case w.PID == self || strings.HasPrefix(app, "jenab"):
		return false, "Jenab itself"
	case denied[app] != "":
		return false, denied[app]
	case isVault(w.Title):
		return false, "a password manager"
	case app == "explorer.exe" && class == "#32770":
		return false, "a system dialog"
	case !w.Visible:
		return false, "not visible"
	}
	return true, ""
}

var (
	// rePassword: a field for a secret, by its name.
	rePassword = regexp.MustCompile(`(?i)pass(word|code|phrase|wort)|\bpin\b|verification code|one-time code|security code|\botp\b|kennwort|geheimzahl|bestätigungscode|sicherheitscode|رمز|گذرواژه|کلمه عبور|کد تأیید|کد تایید|کد امنیتی`)
	// reAccept: a button or link that accepts cookies, terms or other
	// consent ("Accept all", "Alle akzeptieren", "پذیرش همه").
	reAccept = regexp.MustCompile(`(?i)\b(accept|agree|allow all|allow cookies|got it)\b|akzeptier|zustimm|stimme zu|einverstanden|alle zulassen|annehmen|accepter|accetta|aceptar|پذیرش|می‌پذیرم|میپذیرم|موافقم|قبول`)
	// reDecline: declining is allowed, even with a word of reAccept in it
	// ("Reject all", "Alle ablehnen", "Don't accept").
	reDecline = regexp.MustCompile(`(?i)reject|decline|refuse|\bdeny|don'?t|do not|not now|ablehn|verweiger|nicht|رد کردن|رد همه|نمی‌پذیرم|نمیپذیرم|مخالفم`)
	// reCaptcha: a page that asks to prove the user is a person.
	reCaptcha = regexp.MustCompile(`(?i)unusual traffic|not a robot|i'?m not a robot|recaptcha|hcaptcha|captcha|ungewöhnlichen datenverkehr|kein roboter|ربات نیستم|ترافیک غیرعادی`)
	// reVault: a window title that shows a password manager, such as a
	// browser on "Google Password Manager" or a vault's web page.
	reVault = regexp.MustCompile(`(?i)password manager|passwort-?manager|kennwort-?manager|^passwords\b|1password|bitwarden|keepass|lastpass|dashlane|nordpass|proton pass|enpass|roboform|مدیر(یت)? (رمز|گذرواژه)`)
	// reAddress: a browser's address field, by its name.
	reAddress = regexp.MustCompile(`(?i)address|adress|url|نشانی|آدرس`)
)

// isVault: a window title that shows a password manager.
func isVault(title string) bool { return reVault.MatchString(title) }

// isPassword: a field for a secret, by its state or its name.
func isPassword(n Node) bool { return n.has("secure") || rePassword.MatchString(n.Name) }

// isConsent: an element that accepts consent. Only short names count, so
// a paragraph that mentions "accept" isn't one.
func isConsent(n Node) bool {
	return len([]rune(n.Name)) < 60 && reAccept.MatchString(n.Name) && !reDecline.MatchString(n.Name)
}

// isCaptcha: a page that asks to prove the user is a person, such as
// Google's /sorry/ page.
func isCaptcha(page *Node) bool {
	return strings.Contains(page.Value, "/sorry/") || reCaptcha.MatchString(allText(page))
}
