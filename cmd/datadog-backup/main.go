package main

import (
        "github.com/jessevdk/go-flags"
        "github.com/matlockx/datadog-backup/internal"
        "github.com/sirupsen/logrus"
        prefixed "github.com/x-cray/logrus-prefixed-formatter"
        "github.com/zorkian/go-datadog-api"
        "gopkg.in/yaml.v3"
        "io"
        "os"
)

const Pull = "pull"
const Push = "push"
const Delete = "delete"

var ddClient *datadog.Client
var file string = "backup/monitors.yaml"

func main() {

        var opts struct {
                DataDogApiKey  string `long:"api-key" description:"api key for datadog account" required:"true"`
                DataDogAppKey  string `long:"app-key" description:"app key for datadog account" required:"true"`
                Action         string `long:"action" choice:"push" choice:"pull" choice:"delete" description:"push, pull or delete"`
                Sync           bool   `long:"sync" description:"sync config file with datadog"`
                OverrideRemote bool   `long:"override-remote" description:"override remote monitor with local one"`
                DryRun         bool   `long:"dry-run" description:"just show changes"`
                NoBackup       bool   `long:"no-backup" description:"deactivates backup of local file before pulling new content from remote"`
        }
        logrus.SetFormatter(&prefixed.TextFormatter{
                FullTimestamp:   true,
                DisableSorting:  false,
                ForceFormatting: true,
                ForceColors:     true,
        })

        _, err := flags.Parse(&opts)
        fatalOnError(err, "cannot parse args")

        if opts.DryRun {
                logrus.Infof("starting dry run, no changes will be made")
        }

        ddClient = datadog.NewClient(opts.DataDogApiKey, opts.DataDogAppKey)
        backupClient := internal.NewBackupService(ddClient, opts.DryRun, opts.OverrideRemote, !opts.NoBackup)

        switch opts.Action {
        case "push":
                err := backupClient.Push()
                fatalOnError(err, "push")
                err = backupClient.Pull()
                fatalOnError(err, "push-pull")
        case "pull":
                err := backupClient.Pull()
                fatalOnError(err, "pull")
        case "delete":
                deleteMonitors(opts.DryRun)
        }

}

func push(dryRun bool, overrideRemote bool) {
        if overrideRemote {
                logrus.Warnf("remote override active, will override remote monitors")
        }
        var monitors []datadog.Monitor
        monitorsFile := openFile(Push, true)
        decoder := yaml.NewDecoder(monitorsFile)
        err := decoder.Decode(&monitors)
        fatalOnError(err, "push: cannot read monitors file")
        for _, monitor := range monitors {
                var createdMonitor = &monitor
                if !dryRun {
                        if monitor.Name == nil {
                                logrus.Errorf("push: monitor %+v has no name, skipping", monitor)
                                continue
                        }

                        if monitor.Id != nil {
                                remoteMonitor, err := ddClient.GetMonitor(*monitor.Id)
                                if err == nil && remoteMonitor != nil {
                                        if overrideRemote {
                                                err := ddClient.DeleteMonitor(*monitor.Id)
                                                if err != nil {
                                                        logrus.WithError(err).Errorf("push: cannot delete remote monitor %+v", monitor)
                                                        continue
                                                }
                                                logrus.Warnf("push: deleted existing remote monitor with id %d, overriding it with version from file", *monitor.Id)
                                        } else {
                                                logrus.Warnf("push: found existing monitor with id %d, skipping it", *monitor.Id)
                                                continue
                                        }
                                }

                        }

                        remoteMonitors, err := ddClient.GetMonitorsByName(*monitor.Name)
                        if err != nil {
                                logrus.WithError(err).Warnf("push: cannot get monitors with name from %+v, trying to create a new one now", monitor)
                        }
                        if len(remoteMonitors) > 0 {
                                logrus.Warnf("push: monitor %+v has remote monitor with same name, skipping", monitor)
                                continue
                        }

                        createdMonitor, err = ddClient.CreateMonitor(&monitor)
                        if err != nil {
                                logrus.WithError(err).Errorf("push: cannot create monitor %+v, skipping", monitor)
                                continue
                        }
                        logrus.Infof("push: created monitor %#v", createdMonitor)
                }
        }
}

func deleteMonitors(dryRun bool) {

        monitorsFile := openFile(Delete, false)
        defer closeQuietly(monitorsFile)

        var monitors []datadog.Monitor
        decoder := yaml.NewDecoder(monitorsFile)
        err := decoder.Decode(&monitors)
        fatalOnError(err, "delete: cannot read monitors file")
        for _, monitor := range monitors {
                if !dryRun {
                        err = ddClient.DeleteMonitor(*monitor.Id)
                        if err != nil {
                                logrus.WithError(err).Errorf("delete: cannot create monitor %+v", monitor)
                                continue
                        }
                }
                logrus.Infof("deleted monitor %#v", monitor)
        }
}

func pull(dryRun, append bool) {

        monitorsFile := openFile(Pull, append)
        defer closeQuietly(monitorsFile)

        monitors, err := ddClient.GetMonitors()
        fatalOnError(err, "pull: cannot get monitors")

        encoder := yaml.NewEncoder(monitorsFile)
        defer closeQuietly(encoder)

        err = encoder.Encode(monitors)
        fatalOnError(err, "pull: cannot write monitors to file")
}

func openFile(action string, append bool) *os.File {
        fileMode := os.O_RDWR
        switch action {
        case "pull":
                if append {
                        fileMode |= os.O_CREATE | os.O_APPEND
                } else {
                        fileMode |= os.O_CREATE | os.O_TRUNC
                }
        case "push":
        case "delete":
        default:
        }
        monitorsFile, err := os.OpenFile(file, fileMode, 0666)
        fatalOnError(err, "pull: cannot open "+file)
        return monitorsFile
}

func closeQuietly(closer io.Closer) {
        _ = closer.Close()
}

func fatalOnError(err error, msg string) {
        if err != nil {
                logrus.WithError(err).Fatal(msg)
        }
}
