package workers

import (
	"bridgr/internal/app/bridgr"
	"bridgr/internal/app/bridgr/config"
	"bytes"
	"html/template"
	"io"
	"log"
	"os"
	"path"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
)

// Yum is the worker implementation for Yum repositories
type Yum struct {
	Config          *config.Yum
	RepoWriter      io.WriteCloser
	RepoTemplate    string
	PackageMount    mount.Mount
	RepoMount       mount.Mount
	ContainerConfig container.Config
}

// NewYum creates a worker.Yum struct
func NewYum(conf *config.BridgrConf) Worker {
	_ = os.MkdirAll(conf.Yum.BaseDir(), os.ModePerm)
	repo, err := os.Create(path.Join(config.BaseDir(), "bridgr.repo"))
	if err != nil {
		bridgr.Printf("Unable to create YUM repo file: %s", err)
		return nil
	}
	bridgr.Debugf("Created %s for writing repo template", repo.Name())

	return &Yum{
		Config:     &conf.Yum,
		RepoWriter: repo,
		PackageMount: mount.Mount{
			Type:   mount.TypeBind,
			Source: conf.Yum.BaseDir(),
			Target: "/packages",
		},
		RepoMount: mount.Mount{
			Type:   mount.TypeBind,
			Source: repo.Name(),
			Target: "/etc/yum.repos.d/bridgr.repo",
		},
		ContainerConfig: container.Config{
			Image:        conf.Yum.Image.String(),
			Cmd:          []string{"/bin/bash", "-"},
			Tty:          false,
			OpenStdin:    true,
			AttachStdout: true,
			AttachStderr: true,
			StdinOnce:    true,
		},
	}
}

// Name returns the string name of the Yum worker
func (y *Yum) Name() string {
	return "Yum"
}

// Run sets up, creates and fetches a YUM repository based on the settings from the config file
func (y *Yum) Run() error {
	err := y.Setup()
	if err != nil {
		return err
	}
	script, _ := y.script(y.Config.Items)
	hostConfig := container.HostConfig{
		Mounts: []mount.Mount{
			y.PackageMount,
			y.RepoMount,
		},
	}
	return runContainer("bridgr_yum", &y.ContainerConfig, &hostConfig, script)
}

// Setup only does the setup step of the YUM worker
func (y *Yum) Setup() error {
	bridgr.Debug("Called Yum.Setup()")

	err := y.writeRepos()
	if err != nil {
		return err
	}
	return nil
}

func (y *Yum) writeRepos() error {
	yumTmpl, err := loadTemplate("yum.repo")
	if err != nil {
		log.Printf("Error loading yum.repo template: %s", err)
	}
	defer y.RepoWriter.Close()
	tmpl := template.New("yumrepo")
	_, _ = tmpl.Parse(yumTmpl)
	return tmpl.Execute(y.RepoWriter, y.Config.Repos)
}

func (y *Yum) script(packages []string) (string, error) {
	docker, err := loadTemplate("yum.sh")
	if err != nil {
		log.Printf("Error loading yum.sh template: %s", err)
	}
	tmpl := template.New("yumscript")
	tmpl = tmpl.Funcs(template.FuncMap{"Join": strings.Join})
	_, _ = tmpl.Parse(docker)
	final := bytes.Buffer{}
	if err := tmpl.Execute(&final, packages); err != nil {
		return "", err
	}
	return final.String(), nil
}
