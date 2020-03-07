package internal

import (
        "fmt"
        "github.com/pkg/errors"
        "github.com/sirupsen/logrus"
        "github.com/zorkian/go-datadog-api"
        "gopkg.in/yaml.v3"
        "io"
        "io/ioutil"
        "os"
        "time"
)

type backupService struct {
        ddClient       *datadog.Client
        log            *logrus.Entry
        overrideRemote bool
        dryRun         bool
        backup         bool
}

func NewBackupService(ddClient *datadog.Client, dryRun, overrideRemote, backup bool) *backupService {
        return &backupService{
                ddClient:       ddClient,
                log:            logrus.WithField("prefix", "backup-service"),
                overrideRemote: overrideRemote,
                dryRun:         dryRun,
                backup:         backup,
        }
}

func (b *backupService) Pull() error {
        monitorsClient := NewMonitorsClient(b.ddClient)
        return b.pull(monitorsClient)
}

func (b *backupService) Push() error {
        monitorsClient := NewMonitorsClient(b.ddClient)
        return b.push(monitorsClient)
}

func (b *backupService) Delete() error {
        monitorsClient := NewMonitorsClient(b.ddClient)
        return b.delete(monitorsClient)
}

func (b *backupService) push(client DatadogConfigClient) error {
        if b.overrideRemote {
                logrus.Warnf("remote override active, will override remote monitors")
        }

        file, err := b.openConfigFile(b.configFilePath(client.ConfigClientName()), false, true)
        if err != nil {
                return errors.WithMessage(err, "push")
        }
        defer closeQuietly(file)

        configElements, err := client.DecodeFile(file)
        if err != nil {
                return errors.WithMessagef(err, "push: cannot decode file %s", file.Name())
        }

        for _, configElement := range configElements {
                if !b.dryRun {
                        name := configElement.GetName()
                        if name == "" {
                                logrus.Errorf("push: configElement %+v has no name, skipping", configElement.GetDelegate())
                                continue
                        }

                        id := configElement.GetId()
                        if id != -1 {
                                remoteElement, err := client.GetById(id)
                                if err == nil && remoteElement != nil {
                                        if b.overrideRemote {
                                                err := b.ddClient.DeleteMonitor(id)
                                                if err != nil {
                                                        logrus.WithError(err).Errorf("push: cannot delete remote configElement %+v", configElement)
                                                        continue
                                                }
                                                logrus.Warnf("push: deleted existing remote configElement with id %d, overriding it with version from file", id)
                                        } else {
                                                logrus.Warnf("push: found existing configElement with id %d, skipping it", id)
                                                continue
                                        }
                                }
                        }

                        remoteElements, err := client.GetByName(name)
                        if err != nil {
                                logrus.WithError(err).Warnf("push: cannot get monitors with name from %+v, trying to create a new one now", configElement)
                        }
                        if len(remoteElements) > 0 {
                                logrus.Warnf("push: configElement %+v has remote configElement with same name, skipping", configElement)
                                continue
                        }

                        createdElement, err := client.Create(configElement)
                        if err != nil {
                                logrus.WithError(err).Errorf("push: cannot create configElement %+v, skipping", configElement)
                                continue
                        }
                        logrus.Infof("push: created configElement %#v", createdElement)
                }
        }
        return nil
}

func (b *backupService) pull(client DatadogConfigClient) error {

        configFilePath := b.configFilePath(client.ConfigClientName())
        configFile, err := b.openConfigFile(configFilePath, false, true)
        if err != nil {
                return errors.WithMessage(err, "pull")
        }
        defer closeQuietly(configFile)

        if b.backup {
                backupFile := b.configFilePath(fmt.Sprintf("%d_%s", time.Now().Unix(), client.ConfigClientName()))
                err := b.backupFile(configFilePath, backupFile)
                if err != nil {
                        return errors.WithMessage(err, "pull")
                }
        }

        configElements, err := client.GetAll()
        if err != nil {
                return errors.WithMessage(err, "pull")
        }
        b.log.Infof("writing %d config element(s) into configFile %s", len(configElements.Elements), configFile.Name())

        encoder := yaml.NewEncoder(configFile)
        defer closeQuietly(encoder)

        err = encoder.Encode(configElements.Elements)
        return errors.WithMessage(err, "pull")
}

func (b *backupService) backupFile(oldFile, newFile string) error {
        old, err := ioutil.ReadFile(oldFile)
        if err != nil {
                return errors.WithMessagef(err, "backupFile: cannot read config file %s", oldFile)
        }
        err = ioutil.WriteFile(newFile, old, 0644)
        return errors.WithMessage(err, "backup")
}

func (b *backupService) openConfigFile(name string, readOnly, append bool) (*os.File, error) {

        fileMode := os.O_RDWR
        if !readOnly {
                if append {
                        fileMode |= os.O_CREATE | os.O_APPEND
                } else {
                        fileMode |= os.O_CREATE | os.O_TRUNC
                }
        }

        monitorsFile, err := os.OpenFile(name, fileMode, 0666)

        return monitorsFile, errors.WithMessagef(err, "cannot open file %s", name)
}

func (b *backupService) delete(client DatadogConfigClient) error {

        configFile := b.configFilePath(client.ConfigClientName())
        configElements, err := b.readConfigFile(configFile)
        if err != nil {
                return errors.WithMessagef(err, "delete: cannot read config file for %s", configFile)
        }

        for _, configElement := range configElements {
                if !b.dryRun {

                        id := configElement.GetId()
                        if id != -1 {
                                err = b.ddClient.DeleteMonitor(id)
                                if err != nil {
                                        logrus.WithError(err).Errorf("delete: cannot delete monitor %d", id)
                                        continue
                                }
                        } else {
                                logrus.WithError(err).Errorf("delete: cannot delete monitor, id is missing: %+v", configElement.GetDelegate())
                        }

                }
                logrus.Infof("deleted monitor %#v", configElement)
        }
        return nil
}

func (b *backupService) readConfigFile(name string) ([]ConfigElement, error) {
        file, err := b.openConfigFile(name, true, true)
        if err != nil {
                return nil, errors.WithMessage(err, "readConfigFile")
        }
        defer closeQuietly(file)

        var configElements []ConfigElement

        decoder := yaml.NewDecoder(file)
        if err := decoder.Decode(&configElements); err != nil {
                return nil, errors.WithMessage(err, "readConfigFile: cannot read monitors file")
        }
        return configElements, nil
}

func (b *backupService) configFilePath(name string) string {
        return "backup/" + name + ".yaml"
}

func closeQuietly(closer io.Closer) {
        _ = closer.Close()
}
