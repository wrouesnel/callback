//go:build mage

// Self-contained go-project magefile.

// nolint: deadcode,gochecknoglobals,gochecknoinits
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/magefile/mage/target"

	"errors"
	"math/bits"
	"strconv"

	"github.com/integralist/go-findroot/find"
	"github.com/mholt/archiver"
	"io"
)

var curDir = func() string {
	name, _ := os.Getwd()
	return name
}()

const (
	constCoverageDir = ".coverage"
	constToolBinDir  = ".bin"
	constGitHookDir  = "githooks"
	constBinDir      = "bin"
	constReleaseDir  = "release"
	constCmdDir      = "cmd"
	constCoverFile   = ".cover.out"
)

const (
	constManagedScriptSectionHead = "## ++ BUILD SYSTEM MANAGED - DO NOT EDIT ++ ##"
	constManagedScriptSectionFoot = "## -- BUILD SYSTEM MANAGED - DO NOT EDIT -- ##"
)

// normalizePath turns a path into an absolute path and removes symlinks
func normalizePath(name string) string {
	absPath := Must(filepath.Abs(name))
	//return Must(filepath.EvalSymlinks(absPath))
	return absPath
}

// binRootName is set to the name of the directory by default
var binRootName = func() string {
	return Must(find.Repo()).Path
}()

// dockerImageName is set to the name of the directory by default
var dockerImageName = func() string {
	return binRootName
}()

var coverageDir = normalizePath(path.Join(curDir, constCoverageDir))
var toolsBinDir = normalizePath(path.Join(curDir, constToolBinDir))
var gitHookDir = normalizePath(path.Join(curDir, constGitHookDir))
var binDir = normalizePath(path.Join(curDir, constBinDir))
var releaseDir = normalizePath(path.Join(curDir, constReleaseDir))
var cmdDir = normalizePath(path.Join(curDir, constCmdDir))

var outputDirs = []string{binDir, releaseDir, coverageDir}

var containerName = func() string {
	if name := os.Getenv("CONTAINER_NAME"); name != "" {
		return name
	}
	return dockerImageName
}()

type Platform struct {
	OS        string
	Arch      string
	BinSuffix string
}

func (p *Platform) String() string {
	return fmt.Sprintf("%s-%s", p.OS, p.Arch)
}

func (p *Platform) PlatformDir() string {
	platformDir := path.Join(binDir, fmt.Sprintf("%s_%s_%s", productName, versionShort, p.String()))
	return platformDir
}

func (p *Platform) PlatformBin(cmd string) string {
	platformBin := fmt.Sprintf("%s%s", cmd, p.BinSuffix)
	return path.Join(p.PlatformDir(), platformBin)
}

func (p *Platform) ArchiveDir() string {
	return fmt.Sprintf("%s_%s_%s", productName, versionShort, p.String())
}

func (p *Platform) ReleaseBase() string {
	return path.Join(releaseDir, fmt.Sprintf("%s_%s_%s", productName, versionShort, p.String()))
}

// Supported platforms
var platforms []Platform = []Platform{
	{"linux", "arm", ""},
	{"linux", "arm64", ""},
	{"linux", "amd64", ""},
	{"linux", "386", ""},
	{"darwin", "amd64", ""},
	{"darwin", "386", ""},
	{"windows", "amd64", ".exe"},
	{"windows", "386", ".exe"},
	{"freebsd", "amd64", ""},
}

// productName can be overridden by environ product name
var productName = func() string {
	if name := os.Getenv("PRODUCT_NAME"); name != "" {
		return name
	}
	name, _ := os.Getwd()
	return path.Base(name)
}()

// Source files
var goSrc []string
var goDirs []string
var goPkgs []string
var goCmds []string

var version = func() string {
	if v := os.Getenv("VERSION"); v != "" {
		return v
	}
	out, _ := sh.Output("git", "describe", "--dirty")

	if out == "" {
		return "v0.0.0"
	}

	return out
}()

var versionShort = func() string {
	if v := os.Getenv("VERSION_SHORT"); v != "" {
		return v
	}
	out, _ := sh.Output("git", "describe", "--abbrev=0")

	if out == "" {
		return "v0.0.0"
	}

	return out
}()

var concurrency = func() int {
	if v := os.Getenv("CONCURRENCY"); v != "" {
		pv, err := strconv.ParseUint(v, 10, bits.UintSize)
		if err != nil {
			panic(err)
		}
		return int(pv)
	}
	return runtime.NumCPU()
}()

var linterDeadline = func() time.Duration {
	if v := os.Getenv("LINTER_DEADLINE"); v != "" {
		d, _ := time.ParseDuration(v)
		if d != 0 {
			return d
		}
	}
	return time.Second * 60
}()

func Log(args ...interface{}) {
	if mg.Verbose() {
		fmt.Println(args...)
	}
}

func init() {
	// Set environment
	os.Setenv("PATH", fmt.Sprintf("%s:%s", toolsBinDir, os.Getenv("PATH")))
	os.Setenv("GOBIN", toolsBinDir)
	Log("Build PATH: ", os.Getenv("PATH"))
	Log("Concurrency:", concurrency)
	goSrc = func() []string {
		results := new([]string)
		filepath.Walk(".", func(relpath string, info os.FileInfo, err error) error {
			// Ensure absolute path so globs work
			path, err := filepath.Abs(relpath)
			if err != nil {
				panic(err)
			}

			// Look for files
			if info.IsDir() {
				return nil
			}

			// Exclusions
			for _, exclusion := range []string{toolsBinDir, binDir, releaseDir, coverageDir} {
				if strings.HasPrefix(path, exclusion) {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}

			if strings.Contains(path, "/vendor/") {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if strings.Contains(path, ".git") {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if !strings.HasSuffix(path, ".go") {
				return nil
			}

			*results = append(*results, path)
			return nil
		})
		return *results
	}()
	goDirs = func() []string {
		resultMap := make(map[string]struct{})
		for _, path := range goSrc {
			absDir, err := filepath.Abs(filepath.Dir(path))
			if err != nil {
				panic(err)
			}
			resultMap[absDir] = struct{}{}
		}
		results := []string{}
		for k := range resultMap {
			results = append(results, k)
		}
		return results
	}()
	goPkgs = func() []string {
		results := []string{}
		out, err := sh.Output("go", "list", "./...")
		if err != nil {
			panic(err)
		}
		for _, line := range strings.Split(out, "\n") {
			if !strings.Contains(line, "/vendor/") {
				results = append(results, line)
			}
		}
		return results
	}()
	goCmds = func() []string {
		results := []string{}

		finfos, err := ioutil.ReadDir(cmdDir)
		if err != nil {
			panic(err)
		}
		for _, finfo := range finfos {
			results = append(results, finfo.Name())
		}
		return results
	}()

	// Ensure output dirs exist
	for _, dir := range outputDirs {
		os.MkdirAll(dir, os.FileMode(0777))
	}
}

func CopyFile(src, dest string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}

	from, err := os.Open(src)
	if err != nil {
		return err
	}
	defer from.Close()

	to, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, st.Mode())
	if err != nil {
		return err
	}
	defer to.Close()

	if _, err := io.Copy(to, from); err != nil {
		return err
	}

	return nil
}

// Must consumes an error from a function
func Must[T any](result T, err error) T {
	if err != nil {
		panic(err)
	}
	return result
}

// concurrencyLimitedBuild executes a certain number of commands limited by concurrency
func concurrencyLimitedBuild(buildCmds ...interface{}) error {
	resultsCh := make(chan error, len(buildCmds))
	concurrencyControl := make(chan struct{}, concurrency)
	for _, buildCmd := range buildCmds {
		go func(buildCmd interface{}) {
			concurrencyControl <- struct{}{}
			resultsCh <- buildCmd.(func() error)()
			<-concurrencyControl

		}(buildCmd)
	}
	// Doesn't work at the moment
	//	mg.Deps(buildCmds...)
	results := []error{}
	var resultErr error = nil
	for len(results) < len(buildCmds) {
		err := <-resultsCh
		results = append(results, err)
		if err != nil {
			fmt.Println(err)
			resultErr = errors.New("parallel build failed")
		}
		fmt.Printf("Finished %v of %v\n", len(results), len(buildCmds))
	}

	return resultErr
}

// Tools builds build tools of the project and is depended on by all other build targets.
func Tools() (err error) {
	// Catch panics and convert to errors
	defer func() {
		if perr := recover(); perr != nil {
			err = perr.(error)
		}
	}()

	toolBuild := func(toolType string, tools ...string) error {
		toolTargets := []interface{}{}
		for _, toolImport := range tools {
			localToolImport := toolImport
			f := func() error { return sh.Run("go", "install", "-v", localToolImport) }
			toolTargets = append(toolTargets, f)
		}

		Log("Build", toolType, "tools")
		if berr := concurrencyLimitedBuild(toolTargets...); berr != nil {
			return berr
		}
		return nil
	}

	if berr := toolBuild("static", "github.com/golangci/golangci-lint/cmd/golangci-lint@v1.47.2"); berr != nil {
		return berr
	}

	return nil
}

// Lint runs gometalinter for code quality. CI will run this before accepting PRs.
func Lint() error {
	mg.Deps(Tools)
	args := []string{"-j", fmt.Sprintf("%v", concurrency), fmt.Sprintf("--deadline=%s",
		linterDeadline.String()), "run", "--enable-all", "--line-length=120",
		"--disable=gocyclo", "--disable=testify", "--disable=test", "--disable=lll"}
	return sh.RunV("golangci-lint", append(args, goDirs...)...)
}

// fmt runs golangci-lint with the formatter options
func formattingLinter(doFixes bool) error {
	mg.Deps(Tools)
	args := []string{"run", "--disable-all", "--enable=gofmt", "--enable=goimports"}
	if doFixes {
		args = append(args, "--fix")
	}
	return sh.RunV("golangci-lint", args...)
}

// Style checks formatting of the file. CI will run this before acceptiing PRs.
func Style() error {
	return formattingLinter(false)
}

// Fmt automatically formats all source code files
func Fmt() error {
	return formattingLinter(true)
}

func listCoverageFiles() ([]string, error) {
	result := []string{}
	finfos, derr := ioutil.ReadDir(coverageDir)
	if derr != nil {
		return result, derr
	}
	for _, finfo := range finfos {
		result = append(result, path.Join(coverageDir, finfo.Name()))
	}
	return result, nil
}

// Test run test suite
func Test() error {
	mg.Deps(Tools)

	// Ensure coverage directory exists
	if err := os.MkdirAll(coverageDir, os.FileMode(0777)); err != nil {
		return err
	}

	// Clean up coverage directory
	coverFiles, derr := listCoverageFiles()
	if derr != nil {
		return derr
	}
	for _, coverFile := range coverFiles {
		if err := sh.Rm(coverFile); err != nil {
			return err
		}
	}

	// Run tests
	coverProfiles := []string{}
	for _, pkg := range goPkgs {
		coverProfile := path.Join(coverageDir, fmt.Sprintf("%s%s", strings.Replace(pkg, "/", "-", -1), ".out"))
		testErr := sh.Run("go", "test", "-v", "-covermode", "count", fmt.Sprintf("-coverprofile=%s", coverProfile),
			pkg)
		if testErr != nil {
			return testErr
		}
		coverProfiles = append(coverProfiles, coverProfile)
	}

	return nil
}

// Coverage sums up the coverage profiles in .coverage. It does not clean up after itself or before.
func Coverage() error {
	// Clean up coverage directory
	coverFiles, derr := listCoverageFiles()
	if derr != nil {
		return derr
	}

	mergedCoverage, err := sh.Output("gocovmerge", coverFiles...)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(constCoverFile, []byte(mergedCoverage), os.FileMode(0777))
}

// All runs a full suite suitable for CI
func All() error {
	mg.SerialDeps(Style, Lint, Test, Coverage, Release)
	return nil
}

// Release builds release archives under the release/ directory
func Release() error {
	mg.Deps(ReleaseBin)

	for _, platform := range platforms {
		owd, wderr := os.Getwd()
		if wderr != nil {
			return wderr
		}
		os.Chdir(binDir)

		if platform.OS == "windows" {
			// build a zip binary as well
			err := archiver.DefaultZip.Archive([]string{platform.ArchiveDir()}, fmt.Sprintf("%s.zip", platform.ReleaseBase()))
			if err != nil {
				return err
			}
		}
		// build tar gz
		err := archiver.DefaultTarGz.Archive([]string{platform.ArchiveDir()}, fmt.Sprintf("%s.tar.gz", platform.ReleaseBase()))
		if err != nil {
			return err
		}
		os.Chdir(owd)
	}

	return nil
}

func makeBuilder(cmd string, platform Platform) func() error {
	f := func() error {
		cmdSrc := fmt.Sprintf("./%s/%s", Must(filepath.Rel(curDir, cmdDir)), cmd)

		Log("Make platform binary directory:", platform.PlatformDir())
		if err := os.MkdirAll(platform.PlatformDir(), os.FileMode(0777)); err != nil {
			return err
		}

		Log("Checking for changes:", platform.PlatformBin(cmd))
		if changed, err := target.Path(platform.PlatformBin(cmd), goSrc...); !changed {
			if err != nil {
				if !os.IsNotExist(err) {
					return err
				}
			} else {
				return nil
			}
		}

		fmt.Println("Building", platform.PlatformBin(cmd))
		return sh.RunWith(map[string]string{"CGO_ENABLED": "0", "GOOS": platform.OS, "GOARCH": platform.Arch},
			"go", "build", "-a", "-ldflags", fmt.Sprintf("-extldflags '-static' -X main.Version=%s", version),
			"-o", platform.PlatformBin(cmd), cmdSrc)
	}
	return f
}

func getCurrentPlatform() *Platform {
	var curPlatform *Platform
	for _, p := range platforms {
		if p.OS == runtime.GOOS && p.Arch == runtime.GOARCH {
			storedP := p
			curPlatform = &storedP
		}
	}
	Log("Determined current platform:", curPlatform)
	return curPlatform
}

// Binary build a binary for the current platform
func Binary() error {
	curPlatform := getCurrentPlatform()
	if curPlatform == nil {
		return errors.New("current platform is not supported")
	}

	for _, cmd := range goCmds {
		err := makeBuilder(cmd, *curPlatform)()
		if err != nil {
			return err
		}
		// Make a root symlink to the build
		cmdPath := path.Join(curDir, cmd)
		os.Remove(cmdPath)
		if err := os.Symlink(curPlatform.PlatformBin(cmd), cmdPath); err != nil {
			return err
		}
	}

	return nil
}

// ReleaseBin builds cross-platform release binaries under the bin/ directory
func ReleaseBin() error {
	buildCmds := []interface{}{}

	for _, cmd := range goCmds {
		for _, platform := range platforms {
			buildCmds = append(buildCmds, makeBuilder(cmd, platform))
		}
	}

	resultsCh := make(chan error, len(buildCmds))
	concurrencyControl := make(chan struct{}, concurrency)
	for _, buildCmd := range buildCmds {
		go func(buildCmd interface{}) {
			concurrencyControl <- struct{}{}
			resultsCh <- buildCmd.(func() error)()
			<-concurrencyControl

		}(buildCmd)
	}
	// Doesn't work at the moment
	//	mg.Deps(buildCmds...)
	results := []error{}
	var resultErr error = nil
	for len(results) < len(buildCmds) {
		err := <-resultsCh
		results = append(results, err)
		if err != nil {
			fmt.Println(err)
			resultErr = errors.New("parallel build failed")
		}
		fmt.Printf("Finished %v of %v\n", len(results), len(buildCmds))
	}

	return resultErr
}

// Docker builds the docker image
//func Docker() error {
//	mg.Deps(Binary)
//	p := getCurrentPlatform()
//	if p == nil {
//		return errors.New("current platform is not supported")
//	}
//
//	return sh.RunV("docker", "build",
//		fmt.Sprintf("--build-arg=binary=%s",
//			Must(filepath.Rel(curDir, p.PlatformBin(binRootName)))),
//		"-t", containerName, ".")
//}

// Clean deletes build output and cleans up the working directory
func Clean() error {
	for _, name := range goCmds {
		if err := sh.Rm(path.Join(binDir, name)); err != nil {
			return err
		}
	}

	for _, name := range outputDirs {
		if err := sh.Rm(name); err != nil {
			return err
		}
	}
	return nil
}

// Debug prints the value of internal state variables
func Debug() error {
	fmt.Println("Source Files:", goSrc)
	fmt.Println("Packages:", goPkgs)
	fmt.Println("Directories:", goDirs)
	fmt.Println("Command Paths:", goCmds)
	fmt.Println("Output Dirs:", outputDirs)
	fmt.Println("PATH:", os.Getenv("PATH"))
	return nil
}

// Autogen configure local git repository with commit hooks
func Autogen() error {
	fmt.Println("Installing git hooks in local repository...")

	for _, fname := range Must(ioutil.ReadDir(gitHookDir)) {
		hookName := fname.Name()
		if !fname.IsDir() {
			continue
		}

		gitHookPath := fmt.Sprintf(".git/hooks/%s", hookName)
		repoHookPath := path.Join(gitHookDir, fname.Name())

		scripts := []string{}
		for _, scriptName := range Must(ioutil.ReadDir(repoHookPath)) {
			if scriptName.IsDir() {
				continue
			}
			fullHookPath := path.Join(gitHookDir, hookName, scriptName.Name())
			relHookPath := Must(filepath.Rel(binRootName, fullHookPath))

			scripts = append(scripts, relHookPath)

			data := []byte{}
			data, err := ioutil.ReadFile(gitHookPath)
			if err != nil {
				data = []byte("#!/bin/bash\n")
			}

			splitHook := strings.Split(string(data), "\n")
			if strings.TrimRight(splitHook[0], " \t") != "#!/bin/bash" {
				fmt.Printf("Don't know how to update your %s script.\n", hookName)
				return errors.New("unknown pre-commit script format")
			}

			headAt := -1
			tailAt := -1
			for idx, line := range splitHook {
				// Search until header.
				if strings.TrimPrefix(line, " ") == constManagedScriptSectionHead {
					if headAt != -1 {
						fmt.Println("Found multiple managed script sections in ", fname.Name(), "first was at line ", headAt, "second was at line ", idx)
						return errors.New("found multiple managed script sections")
					} else {
						headAt = idx
						continue
					}
				} else if strings.TrimPrefix(line, " ") == constManagedScriptSectionFoot {
					if tailAt != -1 {
						fmt.Println("Found multiple managed script sections in ", fname.Name(), "first was at line ", headAt, "second was at line ", idx)
						return errors.New("found multiple managed script sections")
					} else {
						tailAt = idx + 1
						continue
					}
				}
			}

			if headAt == -1 {
				headAt = 1
			}

			if tailAt == -1 {
				tailAt = len(splitHook)
			}

			scriptPackage := []string{constManagedScriptSectionHead}
			scriptPackage = append(scriptPackage, "# These lines were added by go run mage.go autogen.", "")
			for _, scriptPath := range scripts {
				scriptPackage = append(scriptPackage, fmt.Sprintf("\"./%s\" || exit $?", scriptPath))
			}
			scriptPackage = append(scriptPackage, "", constManagedScriptSectionFoot)

			updatedScript := append(splitHook[:headAt], scriptPackage...)
			updatedScript = append(updatedScript, splitHook[tailAt:]...)

			err = ioutil.WriteFile(gitHookPath, []byte(strings.Join(updatedScript, "\n")),
				os.FileMode(0755))
			if err != nil {
				return err
			}
		}
	}

	return nil
}
