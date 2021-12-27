/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/arduino/go-paths-helper"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	fqbn          string
	compileOutput CompileOutput
)

// compileOutput represents the json returned by the arduino-cli compile command
type CompileOutput struct {
	CompilerOut   string         `json:"compiler_out"`
	CompilerErr   string         `json:"compiler_err"`
	BuilderResult *BuilderResult `json:"builder_result"`
	Success       bool           `json:"success"`
}

type BuilderResult struct {
	BuildPath     string     `json:"build_path"`
	UsedLibraries []*LibInfo `json:"used_libraries"`
}

// coreInfo contains information regarding the core used during the compile process
type CoreInfo struct {
	// corePackager string `json: "CorePackager"`
	CoreName    string `json:"coreName"`
	CoreVersion string `json:"coreVersion"`
}

// LibInfo contains information regarding the library used during the compile process
type LibInfo struct {
	LibName    string `json:"name"`
	LibVersion string `json:"version"`
}

// returnJson contains information regarding the core and libraries used during the compile process
type ReturnJson struct {
	CoreInfo *CoreInfo  `json:"coreInfo"`
	LibsInfo []*LibInfo `json:"libsInfo"`
}

// compileCmd represents the compile command
var compileCmd = &cobra.Command{
	Use:   "compile",
	Short: "Compiles Arduino sketches.",
	Long: `Compiles Arduino sketches outputting an object file and a json file
	The json contains information regarding libraries and core to use to build the full sketch`,
	// Example: , // TODO
	Args: cobra.ExactArgs(1), // the path of the sketch to build
	Run:  compileSketch,
}

func init() {
	rootCmd.AddCommand(compileCmd)
	compileCmd.Flags().StringVarP(&fqbn, "fqbn", "b", "", "Fully Qualified Board Name, e.g.: arduino:avr:uno")
	compileCmd.MarkFlagRequired("fqbn")
}

func compileSketch(cmd *cobra.Command, args []string) {
	logrus.Debug("compile called")

	// let's check the arduino-cli version
	cmdOutput, err := exec.Command("arduino-cli", "version", "--format", "json").Output()
	if err != nil {
		logrus.Warn("Before running this tool be sure to have arduino-cli installed in your $PATH")
		logrus.Fatal(err)
	}
	var unmarshalledOutput map[string]interface{}
	json.Unmarshal(cmdOutput, &unmarshalledOutput)
	logrus.Infof("arduino-cli version: %s", unmarshalledOutput["VersionString"])

	// let's call arduino-cli compile and parse the verbose output
	logrus.Infof("running: arduino-cli compile -b %s %s -v --format json", fqbn, args[0])
	cmdOutput, err = exec.Command("arduino-cli", "compile", "-b", fqbn, args[0], "-v", "--format", "json").Output()
	if err != nil {
		logrus.Fatal(err)
	}
	objFilesPaths, returnJson := parseOutput(cmdOutput)

	workingDir, err := paths.Getwd()
	if err != nil {
		logrus.Fatal(err)
	}
	buildDir := workingDir.Join("build")
	if !buildDir.Exist() {
		if err = buildDir.Mkdir(); err != nil {
			logrus.Fatal(err)
		}
	}

	// Copy the object files from the `<tempdir>/arduino-sketch_stuff/sketch` folder
	for _, objFilePath := range objFilesPaths {
		destObjFilePath := buildDir.Join(objFilePath.Base())
		if err = objFilePath.CopyTo(destObjFilePath); err != nil {
			logrus.Errorf("error copying object file: %s", err)
		} else {
			logrus.Infof("copied file to %s", destObjFilePath)
		}
	}

	// save the result.json in the build dir
	jsonFilePath := buildDir.Join("result.json")
	if jsonContents, err := json.MarshalIndent(returnJson, "", " "); err != nil {
		logrus.Errorf("error serializing json: %s", err)
	} else if err := jsonFilePath.WriteFile(jsonContents); err != nil {
		logrus.Errorf("error writing result.json: %s", err)
	} else {
		logrus.Infof("created new file in: %s", jsonFilePath)
	}
}

// parseOutput function takes cmdOutToParse as argument,
// cmdOutToParse is the output captured from the command run
// the function extract the paths of the .o files and
// a ReturnJson object
func parseOutput(cmdOutToParse []byte) ([]*paths.Path, *ReturnJson) {
	err := json.Unmarshal(cmdOutToParse, &compileOutput)
	if err != nil {
		logrus.Fatal(err)
	} else if !compileOutput.Success {
		logrus.Fatalf("sketch compile was not successful %s", compileOutput.CompilerErr)
	}

	compilerOutLines := strings.Split(compileOutput.CompilerOut, "\n")
	var coreLine string
	for _, compilerOutLine := range compilerOutLines {
		logrus.Debug(compilerOutLine)
		if matched := strings.Contains(compilerOutLine, "Using core"); matched {
			coreLine = compilerOutLine
			break // we should already have coreLine
		}
	}
	if coreLine == "" {
		logrus.Fatal("cannot find core used")
	}

	// this dir contains all the obj files we need (the sketch related ones and not the core or libs)
	sketchDir := paths.New(compileOutput.BuilderResult.BuildPath).Join("sketch")
	sketchFilesPaths, err := sketchDir.ReadDir()
	if err != nil {
		logrus.Fatal(err)
	} else if sketchFilesPaths == nil {
		logrus.Fatalf("empty directory: %s", sketchDir)
	}
	var returnObjectFilesList []*paths.Path
	for _, sketchFilePath := range sketchFilesPaths {
		if sketchFilePath.Ext() == ".o" {
			returnObjectFilesList = append(returnObjectFilesList, sketchFilePath)
		}
	}

	returnJson := ReturnJson{
		CoreInfo: parseCoreLine(coreLine),
		LibsInfo: compileOutput.BuilderResult.UsedLibraries,
	}

	// TODO add also the packager -> maybe ParseReference could be used from the cli
	// TODO core could be calculated from fqbn

	return returnObjectFilesList, &returnJson
}

// parseCoreLine takes the line containig info regarding the core and
// returns a coreInfo object
func parseCoreLine(coreLine string) *CoreInfo {
	words := strings.Split(coreLine, " ")
	strCorePath := words[len(words)-1] // last string has the path of the core
	// maybe check if the path is legit before and logrus.Fatal if not
	corePath := paths.New(strCorePath)
	version := corePath.Base()
	name := corePath.Parent().Base()
	logrus.Debugf("core name: %s, core version: %s", name, version)
	coreInfo := &CoreInfo{
		CoreName:    name,
		CoreVersion: version,
	}
	return coreInfo
}
