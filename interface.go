package config

type Config interface {
	NewSnapshot() interface{}
	SetSnapshot(interface{}, error)
}
