package envs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/symfony-cli/symfony-cli/local/platformsh"
	"github.com/symfony-cli/symfony-cli/util"
)

func (l *Local) relationshipsFromTunnel() Relationships {
	projectRoot := util.RepositoryRootDir(l.Dir)
	envID, err := util.PotentialCurrentEnvironmentID(projectRoot)
	if err != nil {
		if l.Debug {
			fmt.Fprintf(os.Stderr, "WARNING: unable to find the env: %s\n", err)
		}
		return nil
	}
	app := platformsh.GuessSelectedAppByDirectory(l.Dir, platformsh.FindLocalApplications(projectRoot))
	if app == nil {
		if l.Debug {
			fmt.Fprintf(os.Stderr, "WARNING: unable to find the app: %s\n", err)
		}
		return nil
	}

	userHomeDir, err := homedir.Dir()
	if err != nil {
		userHomeDir = ""
	}
	tunnelFile := filepath.Join(userHomeDir, ".platformsh", "tunnel-info.json")
	data, err := ioutil.ReadFile(tunnelFile)
	if err != nil {
		if l.Debug {
			fmt.Fprintf(os.Stderr, "WARNING: unable to read relationships from %s: %s\n", tunnelFile, err)
		}
		return nil
	}
	var tunnel []struct {
		EnvironmentID string                 `json:"environmentId"`
		AppName       string                 `json:"appName"`
		ProjectID     string                 `json:"projectId"`
		Relationship  string                 `json:"relationship"`
		LocalPort     int                    `json:"localPort"`
		Service       map[string]interface{} `json:"service"`
	}
	if err := json.Unmarshal(data, &tunnel); err != nil {
		if l.Debug {
			fmt.Fprintf(os.Stderr, "ERROR: unable to unmarshal tunnel data: %s\n", err)
		}
		return nil
	}
	gitConfig := util.GetProjectConfig(projectRoot, l.Debug)
	if gitConfig == nil {
		if l.Debug {
			fmt.Fprintf(os.Stderr, "WARNING: unable to read Git config: %s\n", err)
		}
		return nil
	}
	rels := make(Relationships)
	for _, config := range tunnel {
		if config.ProjectID == gitConfig.ID && config.EnvironmentID == envID && config.AppName == app.Name {
			config.Service["port"] = strconv.Itoa(config.LocalPort)
			config.Service["host"] = "127.0.0.1"
			config.Service["ip"] = "127.0.0.1"
			rels[config.Relationship] = append(rels[config.Relationship], config.Service)
		}
	}

	if len(rels) > 0 {
		l.Tunnel = envID
		l.TunnelEnv = true
		return rels
	}

	return nil
}

var pathCleaningRegex = regexp.MustCompile("[^a-zA-Z0-9-\\.]+")

type Tunnel struct {
	Dir    string
	Worker string
	Debug  bool
	path   string
}

func (t *Tunnel) IsExposed() bool {
	path, err := t.computePath()
	if err != nil {
		return false
	}
	if _, err := os.Stat(path + "-expose"); err != nil {
		return false
	}
	return true
}

func (t *Tunnel) Expose(expose bool) error {
	path, err := t.computePath()
	if err != nil {
		return err
	}

	if expose {
		file, err := os.Create(path + "-expose")
		if err != nil {
			return err
		}
		return file.Close()
	}

	os.Remove(path + "-expose")
	return nil
}

// Path returns the path to the SymfonyCloud local tunnel state file
func (t *Tunnel) computePath() (string, error) {
	if t.path != "" {
		return t.path, nil
	}
	projectRoot, projectInfo := util.GuessProjectRoot(t.Dir, t.Debug)
	if projectInfo == nil {
		return "", errors.New("unable to get project root")
	}
	envID, err := util.PotentialCurrentEnvironmentID(projectRoot)
	if err != nil {
		return "", errors.Wrap(err, "unable to get current env")
	}
	app := platformsh.GuessSelectedAppByDirectory(t.Dir, platformsh.FindLocalApplications(projectRoot))
	if app == nil {
		return "", errors.New("unable to get current application")
	}
	t.path = getControlFileName(filepath.Join(util.GetHomeDir(), "tunnels"), projectInfo.ID, envID, app.Name, t.Worker)
	return t.path, nil
}

func getControlFileName(dir, projectID, envID, appName, workerName string) string {
	var filename bytes.Buffer

	filename.WriteString(projectID)
	filename.WriteRune('-')
	filename.WriteString(envID)

	if appName != "" {
		filename.WriteString("--")
		filename.WriteString(appName)
	}

	if workerName != "" {
		filename.WriteString("--")
		filename.WriteString(workerName)
	}

	filename.WriteString(".json")

	return filepath.Join(dir, pathCleaningRegex.ReplaceAllString(path.Clean(filename.String()), "-"))
}