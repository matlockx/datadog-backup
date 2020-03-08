package internal

import (
        "github.com/pkg/errors"
        "github.com/sirupsen/logrus"
        "github.com/zorkian/go-datadog-api"
        "gopkg.in/yaml.v3"
        "os"
)

type dashboardsClient struct {
        ddClient *datadog.Client
        log      *logrus.Entry
}

func NewDashboardsClient(ddClient *datadog.Client) DatadogConfigClient {
        return &dashboardsClient{
                ddClient: ddClient,
                log:      logrus.WithField("prefix", "dashboards"),
        }
}

func (d *dashboardsClient) DecodeFile(file *os.File) ([]ConfigElement, error) {
        var configElements []dashboardConfigElement
        decoder := yaml.NewDecoder(file)
        if err := decoder.Decode(&configElements); err != nil {
                return nil, errors.WithMessage(err, "push: cannot read dashboard file")
        }
        result := make([]ConfigElement, len(configElements))
        for i := range configElements {
                result[i] = configElements[i]
        }
        return result, nil
}

func (d *dashboardsClient) GetAll() (*ConfigElements, error) {
        dashboards, err := d.ddClient.GetDashboards()
        if err != nil {
                return nil, errors.WithMessage(err, "get all dashboards")
        }
        result := make([]ConfigElement, len(dashboards))
        fullDashboards := make([]datadog.Dashboard, len(dashboards))

        for e, dashboard := range dashboards {
                fullDashboard, err := d.ddClient.GetDashboard(*dashboard.Id)
                if err != nil {
                        return nil, errors.WithMessagef(err, "cannot fully load dashboard %+v", dashboard)
                }
                result[e] = d.newConfigElement(dashboard.Title, dashboard.Id, fullDashboard)
                fullDashboards[e] = *fullDashboard
        }
        return &ConfigElements{
                Elements: result,
                Delegate: d.toInterfaceSlice(fullDashboards),
        }, errors.WithMessage(err, "get all dashboards")
}

func (d *dashboardsClient) ConfigClientName() string {
        return "dashboards"
}

func (d *dashboardsClient) GetById(id int) (interface{}, error) {
        return d.ddClient.GetDashboard(id)
}

// there is no function to load a dashboard by name
func (d *dashboardsClient) GetByName(name string) ([]interface{}, error) {
        return []interface{}{}, nil
}

func (d *dashboardsClient) Create(e ConfigElement) (interface{}, error) {
        return d.ddClient.CreateDashboard((e.GetDelegate()).(*datadog.Dashboard))
}

func (d *dashboardsClient) Delete(id int) error {
        return d.ddClient.DeleteDashboard(id)
}

func (d *dashboardsClient) toInterfaceSlice(dashboards []datadog.Dashboard) []interface{} {
        result := make([]interface{}, len(dashboards))
        for m := range dashboards {
                result[m] = dashboards[m]
        }
        return result
}

type dashboardConfigElement struct {
        Name     string             `json:"name"`
        Id       int                `json:"id"`
        Delegate *datadog.Dashboard `json:"delegate"`
}

func (d dashboardConfigElement) GetName() string {
        return d.Name
}

func (d dashboardConfigElement) GetId() int {
        return d.Id
}

func (d dashboardConfigElement) GetDelegate() interface{} {
        return d.Delegate
}

func (d *dashboardsClient) newConfigElement(name *string, id *int, value *datadog.Dashboard) ConfigElement {
        n := ""
        if name != nil {
                n = *name
        }
        i := -1
        if id != nil {
                i = *id
        }
        return dashboardConfigElement{
                Name:     n,
                Id:       i,
                Delegate: value,
        }
}
