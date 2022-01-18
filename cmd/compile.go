/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/arduino/go-paths-helper"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var fqbn string

// compileOutput represents the json returned by the arduino-cli compile command
type CompileOutput struct {
	CompilerErr   string         `json:"compiler_err"`
	BuilderResult *BuilderResult `json:"builder_result"`
	Success       bool           `json:"success"`
}

type BuilderResult struct {
	BuildPath     string         `json:"build_path"`
	UsedLibraries []*UsedLibrary `json:"used_libraries"`
	BuildPlatform *BuildPlatform `json:"build_platform"`
}

// UsedLibrary contains information regarding the library used during the compile process
type UsedLibrary struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// BuildPlatform contains information regarding the platform used during the compile process
type BuildPlatform struct {
	Id      string `json:"id"`
	Version string `json:"version"`
}

// ResultJson contains information regarding the core and libraries used during the compile process
type ResultJson struct {
	CoreInfo *BuildPlatform `json:"coreInfo"`
	LibsInfo []*UsedLibrary `json:"libsInfo"`
}

// compileCmd represents the compile command
var compileCmd = &cobra.Command{
	Use:   "compile",
	Short: "Compiles Arduino sketches.",
	Long: `Compiles Arduino sketches outputting an object file and a json file in a build directory
	The json contains information regarding libraries and core to use in order to build the sketch`,
	Example: os.Args[0] + `compile -b arduino:avr:uno /home/umberto/Arduino/Blink`,
	Args:    cobra.ExactArgs(1), // the path of the sketch to build
	Run:     compileSketch,
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

	// let's check if ar is installed on the users machine
	cmdOutput, err = exec.Command("ar", "--version").Output()
	if err != nil {
		logrus.Warn("Before running this tool be sure to have \"GNU ar\" installed in your $PATH")
		logrus.Fatal(err)
	}
	logrus.Infof(strings.Split(string(cmdOutput), "\n")[0]) // print the version of ar

	// check if the path of the sketch passed as args[0] is valid
	sketchPath := paths.New(args[0])
	if !sketchPath.Exist() {
		logrus.Fatalf("the path %s do not exist!", sketchPath)
	}
	var inoPath *paths.Path
	if sketchPath.Ext() == ".ino" {
		inoPath = sketchPath
	} else { // if there are multiple .ino files in the sketchPath we need to know which is the one containing setup() and loop() functions
		files, _ := sketchPath.ReadDir()
		files.FilterSuffix(".ino")
		if len(files) == 0 {
			logrus.Fatal("the sketch path specified does not contain an .ino file")
		} else if len(files) > 1 {
			logrus.Fatalf("the sketch path specified contains multiple .ino files:\n %s \nIn order to make the magic please use the path of the .ino file containing the setup() and loop() functions", strings.Join(files.AsStrings(), "\n"))
		}
		inoPath = files[0]
	}
	logrus.Infof("the ino file path is %s", inoPath)

	// create a main.cpp file in the same dir of the sketch.ino
	// the main.cpp contains the following:
	mainCpp := `
#include "Arduino.h"
void _setup();
void _loop();

void setup() {
_setup();
}

void loop() {
_loop();
}`
	mainCppPath := inoPath.Parent().Join("main.cpp").String()
	err = os.WriteFile(mainCppPath, []byte(mainCpp), 0644)
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.Infof("created %s", mainCppPath)

	// replace setup() with _setup() and loop() with _loop() in the user's sketch.ino file
	// TODO make a backup copy of the sketch and restore it at the end (we have it in input var)
	input, err := ioutil.ReadFile(inoPath.String())
	if err != nil {
		logrus.Fatal(err)
	}
	// TODO this check has meaning??
	if bytes.Contains(input, []byte("_setup()")) {
		logrus.Warnf("already replaced setup() function in %s, skipping", inoPath)
	}
	// TODO this check has meaning??
	if bytes.Contains(input, []byte("_loop()")) {
		logrus.Warnf("already replaced loop() function in %s, skipping", inoPath)
	} else {
		output := bytes.Replace(input, []byte("void setup()"), []byte("void _setup()"), -1)
		output = bytes.Replace(output, []byte("void loop()"), []byte("void _loop()"), -1)
		if err = ioutil.WriteFile(inoPath.String(), output, 0644); err != nil {
			logrus.Fatal(err)
		}
		logrus.Infof("replaced setup() and loop() functions in %s", inoPath)
	}

	// let's call arduino-cli compile and parse the verbose output
	cmdArgs := []string{"compile", "-b", fqbn, inoPath.String(), "-v", "--format", "json"}
	logrus.Infof("running: arduino-cli %s", cmdArgs)
	cmdOutput, err = exec.Command("arduino-cli", cmdArgs...).Output()
	if err != nil {
		logrus.Fatal(err)
	}
	objFilesPaths, returnJson := parseCliCompileOutput(cmdOutput)

	// TODO remove the main.cpp file and restore the sketch ino file

	// we are going to leverage the precompiled library infrastructure to make the linking work.
	// this type of lib, as the type suggest, is already compiled so it only gets linked during the linking phase of a sketch
	// but we have to create a library folder structure in the current directory
	// libsketch
	// ├── examples
	// │   └── sketch
	// │       └── sketch.ino
	// ├── library.properties
	// └── src
	//     ├── cortex-m0plus
	//     │   └── libsketch.a
	//     └── libsketch.h
	libName := strings.ToLower(inoPath.Base())
	workingDir, err := paths.Getwd()
	if err != nil {
		logrus.Fatal(err)
	}
	libDir := workingDir.Join("lib" + libName)
	if libDir.Exist() { // if the dir already exixst we clean it before
		os.RemoveAll(libDir.String())
		logrus.Warn("removed %s", libDir)
	}
	if err = libDir.Mkdir(); err != nil {
		logrus.Fatal(err)
	}

	// run ar to create an archive containing all the object files except the main.cpp.o (we'll create it later)
	// we exclude the main.cpp.o because we are going to link the archive libsjetch.a against another main.cpp
	objFilesPaths.FilterOutPrefix("main.cpp")
	// TODO use the correct name for the archive
	cmdArgs = append([]string{"rcs", buildDir.Join("libsketch.a").String()}, objFilesPaths.AsStrings()...)
	logrus.Infof("running: ar %s", cmdArgs)
	cmdOutput, _ = exec.Command("ar", cmdArgs...).Output()
	logrus.Print(cmdOutput)

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

// parseCliCompileOutput function takes cmdOutToParse as argument,
// cmdOutToParse is the json output captured from the command run
// the function extracts and returns the paths of the .o files
// (generated during the compile phase) and a ReturnJson object
func parseCliCompileOutput(cmdOutToParse []byte) (paths.PathList, *ResultJson) {
	var compileOutput CompileOutput
	err := json.Unmarshal(cmdOutToParse, &compileOutput)
	if err != nil {
		logrus.Fatal(err)
	} else if !compileOutput.Success {
		logrus.Fatalf("sketch compile was not successful: %s", compileOutput.CompilerErr)
	}

	// this dir contains all the obj files we need (the sketch related ones and not the core or libs)
	sketchDir := paths.New(compileOutput.BuilderResult.BuildPath).Join("sketch")
	sketchFilesPaths, err := sketchDir.ReadDir()
	if err != nil {
		logrus.Fatal(err)
	} else if len(sketchFilesPaths) == 0 {
		logrus.Fatalf("empty directory: %s", sketchDir)
	}
	sketchFilesPaths.FilterSuffix(".o")

	returnJson := ResultJson{
		CoreInfo: compileOutput.BuilderResult.BuildPlatform,
		LibsInfo: compileOutput.BuilderResult.UsedLibraries,
	}

	return sketchFilesPaths, &returnJson
}
