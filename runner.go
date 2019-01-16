package gantry // import "github.com/ad-freiburg/gantry"

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/google/shlex"
)

func getContainerExecutable() string {
	if isWharferInstalled() {
		if isUserRoot() || isUserInDockerGroup() {
			return "docker"
		}
		return "wharfer"
	}
	return "docker"
}

func isUserRoot() bool {
	u, err := user.Current()
	if err != nil {
		return false
	}
	return u.Uid == "0"
}

func isUserInDockerGroup() bool {
	u, err := user.Current()
	if err != nil {
		return false
	}
	gids, err := u.GroupIds()
	if err != nil {
		return false
	}
	for _, gid := range gids {
		group, err := user.LookupGroupId(gid)
		if err != nil {
			return false
		}
		if group.Name == "docker" {
			return true
		}
	}
	return false
}

func isWharferInstalled() bool {
	cmd := exec.Command("wharfer", "--version")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

type Executable interface {
	Exec() error
	Output() ([]byte, error)
}

type PrefixedLog struct {
	prefix string
	typ    string
	buf    *bytes.Buffer
}

func NewPrefixedLog(prefix string, typ string) *PrefixedLog {
	return &PrefixedLog{
		prefix: prefix,
		typ:    typ,
		buf:    bytes.NewBuffer([]byte("")),
	}
}

func (l *PrefixedLog) Write(p []byte) (int, error) {
	n, err := l.buf.Write(p)
	if err != nil {
		return n, err
	}
	err = l.Output()
	return n, err
}

func (l *PrefixedLog) Output() error {
	const format string = "\u001b[1m%s\u001b[0m %s\u001b[0m"
	for {
		line, err := l.buf.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if l.typ == "stdout" {
			fmt.Fprintf(os.Stdout, format, l.prefix, line)
		}
		if l.typ == "stderr" {
			fmt.Fprintf(os.Stderr, format, l.prefix, line)
		}
	}
	return nil
}

type Runner interface {
	Executable
	SetCommand(name string, args []string)
}

// Local host
type LocalRunner struct {
	name   string
	args   []string
	prefix string
}

func NewLocalRunner(prefix string) *LocalRunner {
	r := &LocalRunner{
		prefix: prefix,
	}
	return r
}

func (r *LocalRunner) Exec() error {
	cmd := exec.Command(r.name, r.args...)
	stdout := NewPrefixedLog(r.prefix, "stdout")
	stderr := NewPrefixedLog(r.prefix, "stderr")
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (r *LocalRunner) Output() ([]byte, error) {
	cmd := exec.Command(r.name, r.args...)
	return cmd.Output()
}

func (r *LocalRunner) SetCommand(name string, args []string) {
	r.name = name
	r.args = args
}

func NewImageBuilder(step Step) func() error {
	return func() error {
		r := step.Runner()
		r.SetCommand(getContainerExecutable(), []string{"build", "--tag", step.ImageName(), step.BuildInfo.Context})
		return r.Exec()
	}
}

func NewImagePuller(step Step) func() error {
	return func() error {
		r := step.Runner()
		r.SetCommand(getContainerExecutable(), []string{"pull", step.ImageName()})
		return r.Exec()
	}
}

func NewContainerRunner(step Step) func() error {
	return func() error {
		r := step.Runner()
		args := []string{"run", "--name", step.ContainerName()}
		if step.Detach {
			args = append(args, "-d")
		} else {
			args = append(args, "--rm")
		}
		for _, port := range step.Ports {
			args = append(args, "-p", port)
		}
		for _, volume := range step.Volumes {
			// Resolve relative paths
			var err error
			parts := strings.SplitN(volume, ":", 2)
			parts[0], err = filepath.Abs(parts[0])
			if err != nil {
				return err
			}
			args = append(args, "-v", strings.Join(parts, ":"))
		}
		for _, envvar := range step.Environment {
			args = append(args, "-e", envvar)
		}
		// Override entrypoint with step.Command
		callerArgs := step.Args
		if step.Command != "" {
			tokens, _ := shlex.Split(step.Command)
			args = append(args, "--entrypoint", tokens[0])
			callerArgs = tokens[1:]
		}
		args = append(args, step.ImageName())
		args = append(args, callerArgs...)
		r.SetCommand(getContainerExecutable(), args)
		return r.Exec()
	}
}

func NewContainerKiller(step Step) func() error {
	return func() error {
		r := step.Runner()
		r.SetCommand(getContainerExecutable(), []string{"ps", "-q", "--filter", "name=" + step.ContainerName()})
		out, err := r.Output()
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(bytes.NewReader(out))
		scanner.Split(bufio.ScanWords)
		for scanner.Scan() {
			k := step.Runner()
			k.SetCommand(getContainerExecutable(), []string{"kill", scanner.Text()})
			if err := k.Exec(); err != nil {
				return err
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return nil
	}
}

func NewImageExistenceChecker(step Step) func() error {
	return func() error {
		r := step.Runner()
		r.SetCommand(getContainerExecutable(), []string{"images", "--format", "{{.ID}};{{.Repository}}", step.ImageName()})
		out, err := r.Output()
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(bytes.NewReader(out))
		scanner.Split(bufio.ScanWords)
		count := 0
		for scanner.Scan() {
			count++
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("Image not found '%s'", step.ImageName())
		}
		return nil
	}
}

func NewOldContainerRemover(step Step) func() error {
	return func() error {
		r := step.Runner()
		r.SetCommand(getContainerExecutable(), []string{"ps", "-a", "-q", "--filter", "name=" + step.ContainerName()})
		out, err := r.Output()
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(bytes.NewReader(out))
		scanner.Split(bufio.ScanWords)
		for scanner.Scan() {
			k := step.Runner()
			k.SetCommand(getContainerExecutable(), []string{"rm", scanner.Text()})
			if err := k.Exec(); err != nil {
				return err
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return nil
	}
}
