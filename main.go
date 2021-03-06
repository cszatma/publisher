package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cszatma/publisher/config"
	"github.com/cszatma/publisher/fatal"
	"github.com/cszatma/publisher/git"
	"github.com/cszatma/publisher/util"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
)

var (
	configPath string
	skipPreRun bool
	tag        string
	targetName string
	verbose    bool
)

func parseFlags() {
	flag.StringVarP(&configPath, "path", "p", "publisher.yml", "The path to the publisher.yml config file.")
	flag.BoolVar(&skipPreRun, "skip-prerun", false, "Skip preRun step.")
	flag.StringVarP(&tag, "tag", "t", "", "The git tag to create. Omit if you do not want to create a tag.")
	flag.StringVarP(&targetName, "target", "T", "", "The target to deploy.")
	flag.BoolVarP(&verbose, "verbose", "v", false, "Enables verbose logging.")

	flag.Parse()

	if targetName == "" {
		fatal.Exit("Must provider a target to deploy using the --target flag")
	}
}

func main() {
	parseFlags()

	var logLevel log.Level
	if verbose {
		logLevel = log.DebugLevel
	} else {
		logLevel = log.InfoLevel
	}

	log.SetLevel(logLevel)
	log.SetFormatter(&log.TextFormatter{
		DisableTimestamp: true,
	})
	fatal.ShowStackTraces(verbose)

	srcRootPath, err := git.RootDir()
	if err != nil {
		fatal.ExitErr(err, "Project is not a git repo")
	}

	sha, err := git.SHA("HEAD")
	if err != nil {
		fatal.ExitErr(err, "Failed to get SHA of HEAD for project")
	}

	vars := map[string]string{
		"SHA":  sha,
		"TAG":  tag,
		"DATE": time.Now().Local().Format("01-02-2006"),
	}

	log.Debugln("Reading publisher.yml config")
	conf, err := config.Init(configPath, vars)
	if err != nil {
		fatal.ExitErr(err, "Failed to read config file")
	}

	target, ok := conf.Targets[targetName]
	if !ok {
		fatal.Exitf("%s is not a valid deployment target", targetName)
	}

	// Setup target repo
	targetRepoPath := filepath.Join(config.ReposDir(), target.GithubRepo)
	var repo *git.Repository
	if !util.FileOrDirExists(targetRepoPath) {
		log.Debugf("Target repo %s does not exist, cloning...\n", target.GithubRepo)

		repo, err = git.Clone(target.GithubRepo, target.Branch, targetRepoPath)
		if err != nil {
			fatal.ExitErrf(err, "Failed to clone repo %s to %s", target.GithubRepo, targetRepoPath)
		}

		log.Debugf("Successfully cloned repo %s\n", target.GithubRepo)
	} else {
		log.Debugf("Target repo %s exists, opening and setting up \n", target.GithubRepo)

		repo, err = git.Open(target.GithubRepo, target.Branch, targetRepoPath)
		if err != nil {
			fatal.ExitErrf(err, "Failed to open repo %s at path %s", target.GithubRepo, targetRepoPath)
		}

		log.Debugf("Successfully opened repo %s\n", target.GithubRepo)
	}

	// Empty target repo
	log.Debugf("Emptying directory %s\n", targetRepoPath)
	dir, err := ioutil.ReadDir(targetRepoPath)
	if err != nil {
		fatal.ExitErr(err, "failed to read items in target dir")
	}

	for _, f := range dir {
		// Don't remove .git dir
		if f.Name() == ".git" && f.IsDir() {
			continue
		}

		err = os.RemoveAll(filepath.Join(targetRepoPath, f.Name()))
		if err != nil {
			fatal.ExitErrf(err, "failed to remove %s", f.Name())
		}
	}

	if !skipPreRun && conf.PreRunScript != "" {
		fmt.Println("Executing preRun script...")
		args := strings.Split(conf.PreRunScript, " ")
		err = util.Exec(args[0], srcRootPath, args[1:]...)
		if err != nil {
			fatal.ExitErr(err, "Failed to execute preRun script")
		}
	}

	// Copy files
	fmt.Println("Copying files...")
	var files []string
	for _, file := range conf.Files {
		matches, err := filepath.Glob(file)
		if err != nil {
			fatal.ExitErr(err, "failed to parse files listed in config")
		}

		files = append(files, matches...)
	}

	for _, file := range files {
		log.Debugf("Copying %s...\n", file)

		srcPath := filepath.Join(srcRootPath, file)
		var destFile string
		components := strings.Split(file, "/")
		if len(components) == 1 {
			destFile = file
		} else {
			destFile = filepath.Join(components[1:]...)
		}

		destPath := filepath.Join(targetRepoPath, destFile)
		err = util.Copy(srcPath, destPath)
		if err != nil {
			fatal.ExitErrf(err, "failed to copy %s to %s", srcPath, destPath)
		}
	}

	if target.CustomURL != "" {
		cnamePath := filepath.Join(targetRepoPath, "CNAME")
		err = ioutil.WriteFile(cnamePath, []byte(target.CustomURL), 0644)
		if err != nil {
			fatal.ExitErrf(err, "Failed to write CNAME file to target repo %s", targetRepoPath)
		}
	}

	// Commit files
	log.Debugln("Staging files...")
	err = git.Add(target.GithubRepo, targetRepoPath, ".")
	if err != nil {
		fatal.ExitErrf(err, "Failed to stage files in target repo %s", targetRepoPath)
	}

	log.Debugln("Committing files...")
	err = git.Commit(target.GithubRepo, conf.CommitMessage, repo)
	if err != nil {
		fatal.ExitErrf(err, "Failed to commit files in target repo %s", targetRepoPath)
	}

	fmt.Printf("Pushing to branch %s in repo %s\n", target.Branch, target.GithubRepo)
	err = git.Push(target.GithubRepo, repo)
	if err != nil {
		fatal.ExitErrf(err, "Failed to push changes to GitHub for target repo %s", targetRepoPath)
	}

	fmt.Println("Successfully published to GitHub Pages! Enjoy!")
}
