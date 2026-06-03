package memory

// Service is the central memory service used by the engine runtime.
type Service = Manager

func NewService() (*Manager, error) {
	return NewManager()
}

func NewServiceWithPath(basePath string) (*Manager, error) {
	return NewManagerWithPath(basePath)
}
