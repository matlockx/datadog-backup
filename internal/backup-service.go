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
        configClients  []DatadogConfigClient

        configDir string
        backupDir string
}

type BackupConfig struct {
        ConfigDir      string
        BackupDir      string
        DryRun         bool
        OverrideRemote bool
        DoBackup       bool
}

func NewBackupService(ddClient *datadog.Client, config BackupConfig) *backupService {
        service := &backupService{
                ddClient:       ddClient,
                log:            logrus.WithField("prefix", "backup-service"),
                overrideRemote: config.OverrideRemote,
                dryRun:         config.DryRun,
                backup:         config.DoBackup,
                configDir:      config.ConfigDir,
                backupDir:      config.BackupDir,
                configClients: []DatadogConfigClient{
                        NewMonitorsClient(ddClient),
                        NewDashboardsClient(ddClient),
                        NewDowntimesClient(ddClient),
                },
        }
        if _, err := os.Stat(service.configDir); os.IsNotExist(err) {
                service.log.WithError(err).Fatal("config dir does not exist")
        }
        if _, err := os.Stat(service.backupDir); os.IsNotExist(err) && service.backup {
                service.log.WithError(err).Fatal("backup dir does not exist")
        }
        return service
}

func (b *backupService) Pull() error {
        for _, c := range b.configClients {
                if err := b.pull(c); err != nil {
                        return errors.WithMessagef(err, "pull client %s", c.ConfigClientName())
                }
        }
        return nil
}

func (b *backupService) Push() error {
        for _, c := range b.configClients {
                if err := b.push(c); err != nil {
                        return errors.WithMessagef(err, "push client %s", c.ConfigClientName())
                }
        }
        return nil
}

func (b *backupService) Delete() error {
        for _, c := range b.configClients {
                if err := b.delete(c); err != nil {
                        return errors.WithMessagef(err, "delete client %s", c.ConfigClientName())
                }
        }
        return nil
}

func (b *backupService) push(client DatadogConfigClient) error {
        logger := b.log.WithField("client", client.ConfigClientName())
        if b.overrideRemote {
                logger.Warnf("remote override active, will override remote monitors")
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
                name := configElement.GetName()
                if name == "" {
                        logger.Errorf("push: configElement %+v has no name, skipping", configElement.GetDelegate())
                        continue
                }

                id := configElement.GetId()
                if id != -1 {
                        remoteElement, err := client.GetById(id)
                        if err == nil && remoteElement != nil {
                                if b.overrideRemote {
                                        if !b.dryRun {
                                                err := b.ddClient.DeleteMonitor(id)
                                                if err != nil {
                                                        logger.WithError(err).Errorf("push: cannot delete remote configElement %+v", configElement)
                                                        continue
                                                }
                                        }
                                        logger.Warnf("push: deleted existing remote configElement with id %d, overriding it with version from file", id)
                                } else {
                                        logger.Warnf("push: found existing configElement with id %d, skipping it", id)
                                        continue
                                }
                        }
                }

                remoteElements, err := client.GetByName(name)
                if err != nil {
                        logger.WithError(err).Warnf("push: cannot get monitors with name from %+v, trying to create a new one now", configElement)
                }
                if len(remoteElements) > 0 {
                        logger.Warnf("push: configElement %+v has remote configElement with same name, skipping", configElement)
                        continue
                }
                createdElement := configElement.GetDelegate()
                if !b.dryRun {
                        createdElement, err = client.Create(configElement)
                        if err != nil {
                                logger.WithError(err).Errorf("push: cannot create configElement %+v, skipping", configElement)
                                continue
                        }
                }
                logger.Infof("push: created configElement %#v", createdElement)

        }
        return nil
}

func (b *backupService) pull(client DatadogConfigClient) error {
        logger := b.log.WithField("client", client.ConfigClientName())

        if b.backup && !b.dryRun {
                err := b.backupFile(client.ConfigClientName())
                if err != nil {
                        return errors.WithMessage(err, "pull")
                }
        }

        configFilePath := b.configFilePath(client.ConfigClientName())
        configFile, err := b.openConfigFile(configFilePath, false, b.dryRun)
        if err != nil {
                return errors.WithMessage(err, "pull")
        }
        defer closeQuietly(configFile)

        configElements, err := client.GetAll()
        if err != nil {
                return errors.WithMessage(err, "pull")
        }
        logger.Infof("writing %d config element(s) into configFile %s", len(configElements.Elements), configFile.Name())

        if !b.dryRun {
                encoder := yaml.NewEncoder(configFile)
                defer closeQuietly(encoder)

                err = encoder.Encode(configElements.Elements)
                return errors.WithMessage(err, "pull")
        }
        return nil
}

func (b *backupService) delete(client DatadogConfigClient) error {
        logger := b.log.WithField("client", client.ConfigClientName())

        file, err := b.openConfigFile(b.configFilePath(client.ConfigClientName()), true, true)
        if err != nil {
                return errors.WithMessage(err, "delete")
        }
        defer closeQuietly(file)
        configElements, err := client.DecodeFile(file)
        if err != nil {
                return errors.WithMessagef(err, "delete: cannot decode file %s", file.Name())
        }

        for _, configElement := range configElements {

                id := configElement.GetId()
                if id != -1 {
                        if !b.dryRun {
                                err = client.Delete(id)
                                if err != nil {
                                        logger.WithError(err).Errorf("delete: cannot delete element %d", id)
                                        continue
                                }
                        }
                } else {
                        logger.WithError(err).Errorf("delete: cannot delete element, id is missing: %+v", configElement.GetDelegate())
                }
                logger.Infof("deleted element %#v", configElement)
        }
        return nil
}

func (b *backupService) backupFile(configClientName string) error {
        oldFile := fmt.Sprintf("%s/%s.yaml", b.configDir, configClientName)
        backupFile := fmt.Sprintf("%s/%d_%s.yaml", b.backupDir, time.Now().Unix(), configClientName)
        _, err := os.Stat(oldFile)
        if err == nil {
                old, err := ioutil.ReadFile(oldFile)
                if err != nil {
                        return errors.WithMessagef(err, "backupFile: cannot read config file %s", oldFile)
                }
                err = ioutil.WriteFile(backupFile, old, 0644)
                return errors.WithMessage(err, "backup")
        } else if !os.IsNotExist(err) {
                return errors.WithMessage(err, "backup")
        }
        return nil
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
        return b.configDir + "/" + name + ".yaml"
}

func closeQuietly(closer io.Closer) {
        _ = closer.Close()
}
