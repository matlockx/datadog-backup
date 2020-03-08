package internal

import (
        "github.com/pkg/errors"
        "github.com/sirupsen/logrus"
        "github.com/zorkian/go-datadog-api"
        "gopkg.in/yaml.v3"
        "os"
)

type monitorsClient struct {
        ddClient *datadog.Client
        log      *logrus.Entry
}

func (m *monitorsClient) DecodeFile(file *os.File) ([]ConfigElement, error) {
        var configElements []monitorConfigElement
        decoder := yaml.NewDecoder(file)
        if err := decoder.Decode(&configElements); err != nil {
                return nil, errors.WithMessage(err, "push: cannot read monitors file")
        }
        result := make([]ConfigElement, len(configElements))
        for i := range configElements {
                result[i] = configElements[i]
        }
        return result, nil
}

func NewMonitorsClient(ddClient *datadog.Client) DatadogConfigClient {
        return &monitorsClient{
                ddClient: ddClient,
                log:      logrus.WithField("prefix", "monitors"),
        }
}

func (m *monitorsClient) GetAll() (*ConfigElements, error) {
        monitors, err := m.ddClient.GetMonitors()
        if err != nil {
                return nil, errors.WithMessage(err, "get all monitors")
        }
        result := make([]ConfigElement, len(monitors))
        for e, monitor := range monitors {
                result[e] = m.newConfigElement(monitor.Name, monitor.Id, &monitor)
        }
        return &ConfigElements{
                Elements: result,
                Delegate: m.toInterfaceSlice(monitors),
        }, errors.WithMessage(err, "get all monitors")
}

func (m *monitorsClient) ConfigClientName() string {
        return "monitors"
}

func (m *monitorsClient) GetById(id int) (interface{}, error) {
        return m.ddClient.GetMonitor(id)
}

func (m *monitorsClient) GetByName(name string) ([]interface{}, error) {
        monitors, err := m.ddClient.GetMonitorsByName(name)
        return m.toInterfaceSlice(monitors), errors.WithMessage(err, "get monitors by name")
}

func (m *monitorsClient) Create(e ConfigElement) (interface{}, error) {
        return m.ddClient.CreateMonitor((e.GetDelegate()).(*datadog.Monitor))
}

func (m *monitorsClient) Delete(id int) error {
        return m.ddClient.DeleteMonitor(id)
}

func (m *monitorsClient) toInterfaceSlice(monitors []datadog.Monitor) []interface{} {
        result := make([]interface{}, len(monitors))
        for m := range monitors {
                result[m] = monitors[m]
        }
        return result
}

type monitorConfigElement struct {
        Name     string           `json:"name"`
        Id       int              `json:"id"`
        Delegate *datadog.Monitor `json:"delegate"`
}

func (m monitorConfigElement) GetName() string {
        return m.Name
}

func (m monitorConfigElement) GetId() int {
        return m.Id
}

func (m monitorConfigElement) GetDelegate() interface{} {
        return m.Delegate
}

func (m *monitorsClient) newConfigElement(name *string, id *int, value *datadog.Monitor) ConfigElement {
        n := ""
        if name != nil {
                n = *name
        }
        i := -1
        if id != nil {
                i = *id
        }
        return monitorConfigElement{
                Name:     n,
                Id:       i,
                Delegate: value,
        }
}
