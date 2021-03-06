package engines

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"

	log "github.com/Sirupsen/logrus"
	"github.com/camptocamp/conplicity/handler"
	"github.com/camptocamp/conplicity/metrics"
	"github.com/camptocamp/conplicity/util"
	"github.com/camptocamp/conplicity/volume"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// ResticEngine implements a backup engine with Restic
type ResticEngine struct {
	Handler *handler.Conplicity
	Volume  *volume.Volume
}

// GetName returns the engine name
func (*ResticEngine) GetName() string {
	return "Restic"
}

// Backup performs the backup of the passed volume
func (r *ResticEngine) Backup() (err error) {

	v := r.Volume

	targetURL, err := url.Parse(v.Config.TargetURL)
	if err != nil {
		err = fmt.Errorf("failed to parse target URL: %v", err)
		return
	}

	v.Target = targetURL.String()
	v.BackupDir = v.Mountpoint + "/" + v.BackupDir
	v.Mount = v.Name + ":" + v.Mountpoint + ":ro"

	err = util.Retry(3, r.init)
	if err != nil {
		err = fmt.Errorf("failed to create a secure bucket: %v", err)
		return
	}

	err = util.Retry(3, r.resticBackup)
	if err != nil {
		err = fmt.Errorf("failed to backup the volume: %v", err)
		return
	}

	if _, err := r.Handler.IsCheckScheduled(v); err == nil {
		err = util.Retry(3, r.verify)
		if err != nil {
			err = fmt.Errorf("failed to verify backup: %v", err)
			return err
		}
	}
	return
}

// init initialize a secure bucket
func (r *ResticEngine) init() (err error) {
	v := r.Volume
	state, stdout, err := r.launchRestic(
		[]string{
			"-r",
			v.Target,
			"init",
		},
		[]string{
			v.Mount,
		},
	)
	if strings.Contains(stdout, "already initialized") {
		err = nil
		return
	}
	if err != nil {
		err = fmt.Errorf("failed to launch Restic to initialize the repository: %v", err)
		return
	}
	if state != 0 {
		err = fmt.Errorf("Restic existed with state %v while initializing repository", state)
		return
	}
	return
}

// resticBackup performs the backup of a volume with Restic
func (r *ResticEngine) resticBackup() (err error) {
	v := r.Volume
	state, _, err := r.launchRestic(
		[]string{
			"-r",
			v.Target,
			"backup",
			v.BackupDir,
		},
		[]string{
			v.Mount,
		},
	)
	if err != nil {
		err = fmt.Errorf("failed to launch Restic to backup the volume: %v", err)
	}
	if state != 0 {
		err = fmt.Errorf("Restic exited with state %v while backuping the volume", state)
	}
	return
}

// verify checks that the backup is usable
func (r *ResticEngine) verify() (err error) {
	v := r.Volume
	state, _, err := r.launchRestic(
		[]string{
			"-r",
			v.Target,
			"check",
		},
		[]string{
			v.Mount,
		},
	)
	if err != nil {
		err = fmt.Errorf("failed to launch Restic to check the backup: %v", err)
		return
	}
	if state == 0 {
		now := time.Now().Local()
		os.Chtimes(v.Mountpoint+"/.conplicity_last_check", now, now)
	} else {
		err = fmt.Errorf("Restic exited with state %v while checking the backup", state)
	}

	metric := r.Volume.MetricsHandler.NewMetric("conplicity_verifyExitCode", "gauge")
	err = metric.UpdateEvent(
		&metrics.Event{
			Labels: map[string]string{
				"volume": v.Name,
			},
			Value: strconv.Itoa(state),
		},
	)
	return
}

// launchRestic starts a restic container with the given command and binds
func (r *ResticEngine) launchRestic(cmd, binds []string) (state int, stdout string, err error) {
	err = util.PullImage(r.Handler.Client, r.Handler.Config.Restic.Image)
	if err != nil {
		err = fmt.Errorf("failed to pull image: %v", err)
		return
	}

	env := []string{
		"AWS_ACCESS_KEY_ID=" + r.Handler.Config.AWS.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + r.Handler.Config.AWS.SecretAccessKey,
		"OS_USERNAME=" + r.Handler.Config.Swift.Username,
		"OS_PASSWORD=" + r.Handler.Config.Swift.Password,
		"OS_AUTH_URL=" + r.Handler.Config.Swift.AuthURL,
		"OS_TENANT_NAME=" + r.Handler.Config.Swift.TenantName,
		"OS_REGION_NAME=" + r.Handler.Config.Swift.RegionName,
		"RESTIC_PASSWORD=" + r.Handler.Config.Restic.Password,
	}

	log.WithFields(log.Fields{
		"image":       r.Handler.Config.Restic.Image,
		"command":     strings.Join(cmd, " "),
		"environment": strings.Join(env, ", "),
		"binds":       strings.Join(binds, ", "),
	}).Debug("Creating container")

	container, err := r.Handler.ContainerCreate(
		context.Background(),
		&container.Config{
			Cmd:          cmd,
			Env:          env,
			Image:        r.Handler.Config.Restic.Image,
			OpenStdin:    true,
			StdinOnce:    true,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			Tty:          true,
		},
		&container.HostConfig{
			Binds: binds,
		}, nil, "",
	)
	if err != nil {
		err = fmt.Errorf("failed to create container: %v", err)
		return
	}
	defer util.RemoveContainer(r.Handler.Client, container.ID)

	log.Debugf("Launching 'restic %v'...", strings.Join(cmd, " "))
	err = r.Handler.ContainerStart(context.Background(), container.ID, types.ContainerStartOptions{})
	if err != nil {
		err = fmt.Errorf("failed to start container: %v", err)
		return
	}
	var exited bool

	for !exited {
		var cont types.ContainerJSON
		cont, err = r.Handler.ContainerInspect(context.Background(), container.ID)
		if err != nil {
			err = fmt.Errorf("failed to inspect container: %v", err)
			return
		}
		if cont.State.Status == "exited" {
			exited = true
			state = cont.State.ExitCode
		}
	}

	body, err := r.Handler.ContainerLogs(context.Background(), container.ID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Details:    true,
		Follow:     true,
	})
	if err != nil {
		err = fmt.Errorf("failed to retrieve logs: %v", err)
		return
	}
	defer body.Close()
	content, err := ioutil.ReadAll(body)
	if err != nil {
		err = fmt.Errorf("failed to read logs from response: %v", err)
		return
	}
	stdout = string(content)
	log.Debug(stdout)

	return
}
