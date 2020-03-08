package main

import (
        "github.com/jessevdk/go-flags"
        "github.com/matlockx/datadog-backup/internal"
        "github.com/sirupsen/logrus"
        prefixed "github.com/x-cray/logrus-prefixed-formatter"
        "github.com/zorkian/go-datadog-api"
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
                ConfigDir      string `long:"config-dir" default:"config" description:"config directory of monitors, dashboards, etc "`
                BackupDir      string `long:"backup-dir" default:"backup" description:"backup dir for configs where to backup the old config file before pulling new entries from datadog"`
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
        backupClient := internal.NewBackupService(ddClient, internal.BackupConfig{
                ConfigDir:      opts.ConfigDir,
                BackupDir:      opts.BackupDir,
                DryRun:         opts.DryRun,
                OverrideRemote: opts.OverrideRemote,
                DoBackup:       !opts.NoBackup,
        })

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
                err := backupClient.Delete()
                fatalOnError(err, "delete")
        }

}

func fatalOnError(err error, msg string) {
        if err != nil {
                logrus.WithError(err).Fatal(msg)
        }
}
