//go:build windows

package startup

// NewProductLogonTask scopes the shared product login trigger separately from WinTray.
func NewProductLogonTask() (*LogonTask, error) {
	task, err := NewLogonTask()
	if err != nil {
		return nil, err
	}
	task.name = "WinOne-" + task.userSID
	return task, nil
}
