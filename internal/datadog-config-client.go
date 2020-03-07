package internal

import "os"

type DatadogConfigClient interface {
        ConfigClientName() string
        DecodeFile(file *os.File) ([]ConfigElement, error)
        GetAll() (*ConfigElements, error)
        GetById(id int) (interface{}, error)
        GetByName(name string) ([]interface{}, error)

        Create(e ConfigElement) (interface{}, error)
        Delete(id int) error
}

type ConfigElements struct {
        Elements []ConfigElement
        Delegate []interface{}
}

type ConfigElement interface {
        GetName() string
        GetId() int
        GetDelegate() interface{}
}
