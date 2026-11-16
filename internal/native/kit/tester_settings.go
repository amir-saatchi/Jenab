package kit

import "github.com/egoist/mygo/ui"

// More keys the settings' tests press (TASK-055).
const (
	KeySpace = ui.KeySpace
	KeyRight = ui.KeyRight
	KeyLeft  = ui.KeyLeft
	KeyEnd   = ui.KeyEnd
	KeyA     = ui.KeyA
)

// More roles HasRole knows, for the settings' tests.
func init() {
	roles["table"] = ui.RoleTable
	roles["columnheader"] = ui.RoleColumnHeader
	roles["switch"] = ui.RoleSwitch
	roles["group"] = ui.RoleGroup
	roles["dialog"] = ui.RoleDialog
	roles["alertdialog"] = ui.RoleAlertDialog
	roles["progress"] = ui.RoleProgress
}
