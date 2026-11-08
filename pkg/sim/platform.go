package sim

// Driver performs interaction on one device. Implementations: adb, phoneharness.
type Driver interface {
	Tap(x, y int) error
	Type(text string) error
	Swipe(x1, y1, x2, y2, ms int) error
	Key(key string) error
	Back() error
	UI() ([]Node, error)
	Screenshot() ([]byte, error)
	ScreenSize() (w, h int, err error)
	Shell(cmd string) (string, error)
}

// Device is one attached emulator or phone.
type Device struct {
	Serial string `json:"serial"`
	State  string `json:"state"`
	Info   string `json:"info,omitempty"`
}

// BootOpts configures Platform.Boot.
type BootOpts struct {
	Name    string // AVD / simulator name
	Create  bool
	Timeout int // seconds
}

// Tool is one entry in a Check report.
type Tool struct {
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Version  string `json:"version,omitempty"`
	OK       bool   `json:"ok"`
	Optional bool   `json:"optional,omitempty"`
	Note     string `json:"note,omitempty"`
}

// Platform manages a device family (Android today; iOS can be added beside it).
type Platform interface {
	Name() string
	Check() []Tool
	Boot(BootOpts) (serial string, err error)
	Connect(target string, device bool, serial string) error
	Devices() ([]Device, error)
	Install(path string, grant bool) error
	Launch(app string) (component string, err error)
	Stop(pkg string) error
	Clear(pkg string) error
	Grant(pkg string, perms []string) error
	Shell(cmd string) (string, error)
	Driver(name string) (Driver, error)
}
