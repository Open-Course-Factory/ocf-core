package hooks

// BaseHook carries the five identity accessors every Hook must expose.
// Embed it and set the fields in the constructor; only Execute is left to write.
type BaseHook struct {
	Name       string
	EntityName string
	HookTypes  []HookType
	Enabled    bool
	Priority   int
}

func (b *BaseHook) GetName() string          { return b.Name }
func (b *BaseHook) GetEntityName() string    { return b.EntityName }
func (b *BaseHook) GetHookTypes() []HookType { return b.HookTypes }
func (b *BaseHook) IsEnabled() bool          { return b.Enabled }
func (b *BaseHook) GetPriority() int         { return b.Priority }
