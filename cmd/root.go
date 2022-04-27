/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"context"
	"fmt"
	"github.com/manifoldco/promptui"
	"github.com/rotisserie/eris"
	changelogdocutils "github.com/solo-io/go-utils/changeloggenutils"
	"github.com/solo-io/go-utils/githubutils"
	"github.com/solo-io/go-utils/log"
	"github.com/solo-io/go-utils/versionutils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

type Options struct {
	Dir  string
	Org  string
	Repo string
}

var opts Options

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "create-changelog",
	Short: "interactive command to create a changelog dir / file",
	Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	RunE: Run,
}

type promptContent struct {
	errorMsg string
	label    string
}

func promptGetSelect(pc promptContent, items []string) string {
	index := -1
	var result string
	var err error

	for index < 0 {
		prompt := promptui.Select{
			Label: pc.label,
			Items: items,
		}

		index, result, err = prompt.Run()

		if index == -1 {
			items = append(items, result)
		}
	}

	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Input: %s\n", result)

	return result
}

func promptGetInput(pc promptContent) string {
	validate := func(input string) error {
		if len(input) <= 0 {
			return eris.New(pc.errorMsg)
		}
		return nil
	}

	templates := &promptui.PromptTemplates{
		Prompt:  "{{ . }} ",
		Valid:   "{{ . | green }} ",
		Invalid: "{{ . | red }} ",
		Success: "{{ . | bold }} ",
	}

	prompt := promptui.Prompt{
		Label:     pc.label,
		Templates: templates,
		Validate:  validate,
	}

	result, err := prompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Input: %s\n", result)

	return result
}

func Run(cmd *cobra.Command, args []string) error {
	fileInfo, err := os.Stat(opts.Dir)
	if err != nil {
		return err
	}
	if !fileInfo.IsDir() {
		return eris.Errorf("argument dir '%s' is not a directory. Expected a changelog directory.", opts.Dir)
	}
	fmt.Printf("Using directory %s as changelog directory\n", opts.Dir)
	fileInfos, err := ioutil.ReadDir(opts.Dir)
	if err != nil {
		return err
	}

	return GetVersions(fileInfos)
}

func GetVersions(infos []fs.FileInfo) error {
	var versions []versionutils.Version
	for _, fileInfo := range infos {
		name := fileInfo.Name()
		//if strings.HasPrefix(name, "v") {
		//  name = name[1:]
		//}
		if fileInfo.IsDir() {
			v, err := versionutils.ParseVersion(name)
			if err != nil {
				log.Printf("dir %s is not valid semver: %s", name, err.Error())
				continue
			}
			versions = append(versions, *v)
		}
	}
	changelogdocutils.SortReleaseVersions(versions)
	if len(versions) == 0 {
		return eris.New("no valid semver directories found in changelog directory, create at least one.")
	}
	fmt.Printf("Found latest changelog version %s\n", versions[0].String())
	createChangelogPrompt := fmt.Sprintf("Create new changelog file in directory %s", versions[0].String())
	answer := promptGetSelect(promptContent{
		label:    "What would you like to do?",
		errorMsg: "please select a valid input",
	}, []string{createChangelogPrompt, "Check github for latest releases (In case the release I listed isn't the correct minor version)", "Create my own changelog directory"})
	if answer == createChangelogPrompt {
		err := CreateChangelogFile(path.Join(opts.Dir, versions[0].String()))
		if err != nil {
			return err
		}
		return nil
	} else if answer == "Check github for latest release" {
		if len(opts.Org) == 0 || len(opts.Repo) == 0 {
			return eris.New("please use the -r and -o flag to specify the github repo and org to check github , e.g. -o solo.io -r gloo")
		}
		ctx := context.Background()
		client, err := githubutils.GetClient(ctx)
		if err != nil {
			return err
		}
		releases, err := githubutils.GetAllRepoReleasesWithMax(context.Background(), client, opts.Org, opts.Repo, 10)
		if err != nil {
			return eris.Wrapf(err, "error fetching github releases from repo github.com/%s/%s", opts.Org, opts.Repo)
		}
		githubutils.SortReleasesBySemver(releases)
		var r []string
		for _, release := range releases {
			r = append(r, release.GetTagName())
		}
		fmt.Printf("From github.com/%s/%s, the latest releases are \n-%s\n", opts.Org, opts.Repo, strings.Join(r, "\n-"))
		return nil
	} else if answer == "Create my own changelog directory" {
		dir := promptGetInput(promptContent{
			label:    "What is the name of the directory you would like to create? Please enter valid semver",
			errorMsg: "Please enter valid input",
		})
		err := CreateDir(dir)
		if err != nil {
			return err
		}
	}
	return nil
}

func CreateDir(versionDir string) error {
	yOrN := promptGetSelect(promptContent{
		"Please provide y / n",
		fmt.Sprintf("Going to create changelog directory %s with an empty changelog file, is that ok?", versionDir),
	}, []string{"y", "n"})
	if yOrN != "y" {
		joined := path.Join(opts.Dir, versionDir)
		//err := ioutil.WriteFile(fmt.Sprintf("%s/changelog.yaml", joined), []byte(""), 0644)
		//if err != nil {
		//  return eris.Wrapf(err, "error creating directory %s", joined)
		//}
		err := CreateChangelogFile(joined)
		if err != nil {
			return err
		}
	} else if yOrN == "n" {
		fmt.Println("not creating changelog directory, returning you to main prompt.")
		return nil
	} else {
		fmt.Println("unable to understand your input, please provide a valid input y or n")
		return CreateDir(versionDir)
	}
	return nil
}

func CreateChangelogFile(dir string) error {
	changelogFile := promptGetInput(promptContent{
		label:    "What would you like to call the changelog file? Include the .yaml suffix",
		errorMsg: "Please provide valid input",
	})

	newFileName := path.Join(dir, changelogFile)
	if err := ioutil.WriteFile(newFileName, []byte("changelog:\n  -"), 0644); err != nil {
		return eris.Wrapf(err, "error writing changelog file %s", newFileName)
	}
	fmt.Printf("Successfully created changelog file %s\n", newFileName)
	return nil
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	//rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.create-changelog.yaml)")
	rootCmd.PersistentFlags().StringVarP(&opts.Dir, "dir", "d", ".", "changelog directory -- ./changelog, .")
	rootCmd.PersistentFlags().StringVarP(&opts.Org, "org", "o", "", "github organization / user name, e.g. solo-io")
	rootCmd.PersistentFlags().StringVarP(&opts.Repo, "repo", "r", "", "github repo / project, e.g. gloo")

	// Cobra also supports local flags, which will only run

	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {

	viper.AutomaticEnv() // read in environment variables that match

}
