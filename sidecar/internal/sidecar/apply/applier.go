package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"filebeat-k8s/internal/control"
)

const stateFilename = ".fbctl-state.json"

type Applier struct {
	inputsDir string
}

type State struct {
	Checksum  string      `json:"checksum"`
	AppliedAt time.Time   `json:"applied_at"`
	Files     []StateFile `json:"files"`
}

type StateFile struct {
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
}

func New(inputsDir string) *Applier {
	return &Applier{inputsDir: inputsDir}
}

func (a *Applier) LoadLastChecksum() string {
	state, err := a.LoadState()
	if err != nil {
		return ""
	}
	return state.Checksum
}

func (a *Applier) LoadState() (State, error) {
	body, err := os.ReadFile(filepath.Join(a.inputsDir, stateFilename))
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(body, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (a *Applier) Apply(resp control.DesiredConfigResponse) error {
	localChecksum := control.ConfigSetChecksum(resp.Files)
	if localChecksum != resp.Checksum {
		return fmt.Errorf("checksum mismatch: server=%s local=%s", resp.Checksum, localChecksum)
	}
	if err := os.MkdirAll(a.inputsDir, 0755); err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, file := range resp.Files {
		if err := control.ValidateManagedFilename(file.Filename); err != nil {
			return err
		}
		keep[file.Filename] = true
		if err := a.writeAtomic(file); err != nil {
			return err
		}
	}
	if err := a.removeOrphans(keep); err != nil {
		return err
	}
	if err := a.writeState(resp); err != nil {
		return err
	}
	return syncDir(a.inputsDir)
}

func (a *Applier) writeAtomic(file control.ConfigFile) error {
	tmp := filepath.Join(a.inputsDir, "."+file.Filename+".tmp")
	final := filepath.Join(a.inputsDir, file.Filename)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(file.Content); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

func (a *Applier) removeOrphans(keep map[string]bool) error {
	matches, err := filepath.Glob(filepath.Join(a.inputsDir, "fbctl-*.yml"))
	if err != nil {
		return err
	}
	for _, match := range matches {
		name := filepath.Base(match)
		if !keep[name] {
			if err := os.Remove(match); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Applier) writeState(resp control.DesiredConfigResponse) error {
	state := State{Checksum: resp.Checksum, AppliedAt: time.Now().UTC()}
	files := append([]control.ConfigFile(nil), resp.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Filename < files[j].Filename })
	for _, f := range files {
		state.Files = append(state.Files, StateFile{Filename: f.Filename, SHA256: control.ContentChecksum(f.Content)})
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(a.inputsDir, "."+stateFilename+".tmp")
	final := filepath.Join(a.inputsDir, stateFilename)
	if err := os.WriteFile(tmp, append(body, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer f.Close()
	_ = f.Sync()
	return nil
}
