package internal

import (
        "github.com/pkg/errors"
        "github.com/sirupsen/logrus"
        "github.com/zorkian/go-datadog-api"
        "gopkg.in/yaml.v3"
        "os"
)

type downtimesClient struct {
        ddClient *datadog.Client
        log      *logrus.Entry
}

func NewDowntimesClient(ddClient *datadog.Client) DatadogConfigClient {
        return &downtimesClient{
                ddClient: ddClient,
                log:      logrus.WithField("prefix", "downtimes"),
        }
}

func (d *downtimesClient) DecodeFile(file *os.File) ([]ConfigElement, error) {
        var configElements []downtimeConfigElement
        decoder := yaml.NewDecoder(file)
        if err := decoder.Decode(&configElements); err != nil {
                return nil, errors.WithMessage(err, "push: cannot read downtimes file")
        }
        result := make([]ConfigElement, len(configElements))
        for i := range configElements {
                result[i] = configElements[i]
        }
        return result, nil
}

func (d *downtimesClient) GetAll() (*ConfigElements, error) {
        downtimes, err := d.ddClient.GetDowntimes()
        if err != nil {
                return nil, errors.WithMessage(err, "get all downtimes")
        }
        result := make([]ConfigElement, len(downtimes))
        for e, downtime := range downtimes {
                result[e] = d.newConfigElement(downtime.Message, downtime.Id, &downtime)
        }
        return &ConfigElements{
                Elements: result,
                Delegate: d.toInterfaceSlice(downtimes),
        }, errors.WithMessage(err, "get all downtimes")
}

func (d *downtimesClient) ConfigClientName() string {
        return "downtimes"
}

func (d *downtimesClient) GetById(id int) (interface{}, error) {
        return d.ddClient.GetDowntime(id)
}

// there is no function to load a downtime by name
func (d *downtimesClient) GetByName(name string) ([]interface{}, error) {
        return []interface{}{}, nil
}

func (d *downtimesClient) Create(e ConfigElement) (interface{}, error) {
        return d.ddClient.CreateDowntime((e.GetDelegate()).(*datadog.Downtime))
}

func (d *downtimesClient) Delete(id int) error {
        return d.ddClient.DeleteDowntime(id)
}

func (d *downtimesClient) toInterfaceSlice(dashboards []datadog.Downtime) []interface{} {
        result := make([]interface{}, len(dashboards))
        for m := range dashboards {
                result[m] = dashboards[m]
        }
        return result
}

type downtimeConfigElement struct {
        Name     string            `json:"name"`
        Id       int               `json:"id"`
        Delegate *datadog.Downtime `json:"delegate"`
}

func (d downtimeConfigElement) GetName() string {
        return d.Name
}

func (d downtimeConfigElement) GetId() int {
        return d.Id
}

func (d downtimeConfigElement) GetDelegate() interface{} {
        return d.Delegate
}

func (d *downtimesClient) newConfigElement(name *string, id *int, value *datadog.Downtime) ConfigElement {
        n := ""
        if name != nil {
                n = *name
        }
        i := -1
        if id != nil {
                i = *id
        }
        return downtimeConfigElement{
                Name:     n,
                Id:       i,
                Delegate: value,
        }
}
