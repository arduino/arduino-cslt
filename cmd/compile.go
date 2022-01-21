/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"bytes"
	"encoding/json"
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
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	ProvidesIncludes []string `json:"provides_includes"`
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

	// let's check if gcc-ar version
	cmdOutput, err = exec.Command("gcc-ar", "--version").CombinedOutput()
	if err != nil {
		logrus.Warn("Before running this tool be sure to have \"gcc-ar\" installed in your $PATH")
		logrus.Fatal(err)
	}
	// print the version of ar
	logrus.Infof(strings.Split(string(cmdOutput), "\n")[0])

	// check if the path of the sketch passed as args[0] is valid and get the path of the main sketch.ino (in case the sketch dir is specified)
	inoPath := getInoSketchPath(args[0])

	// create a main.cpp file in the same dir of the sketch.ino
	createMainCpp(inoPath)

	// replace setup() with _setup() and loop() with _loop() in the user's sketch.ino file
	oldSketchContent := patchSketch(inoPath)

	// let's call arduino-cli compile and parse the verbose output
	cmdArgs := []string{"compile", "-b", fqbn, inoPath.String(), "-v", "--format", "json"}
	logrus.Infof("running: arduino-cli %s", strings.Join(cmdArgs, " "))
	cmdOutput, err = exec.Command("arduino-cli", cmdArgs...).Output()
	if err != nil {
		logrus.Fatal(err)
	}

	objFilePaths, returnJson := parseCliCompileOutput(cmdOutput)

	// this is done to get the {build.mcu} used later to create the lib dir structure
	// the --show-properties will only print on stdout and not compile
	// the json output is currently broken with this flag, see https://github.com/arduino/arduino-cli/issues/1628
	cmdArgs = []string{"compile", "-b", fqbn, inoPath.String(), "--show-properties"}
	logrus.Infof("running: arduino-cli %s", strings.Join(cmdArgs, " "))
	cmdOutput, err = exec.Command("arduino-cli", cmdArgs...).Output()
	if err != nil {
		logrus.Fatal(err)
	}

	buildMcu := parseCliCompileOutputShowProp(cmdOutput)

	// remove main.cpp file, we don't need it anymore
	removeMainCpp(inoPath)

	// restore the sketch content, this allows us to rerun cslt-tool if we want without breaking the compile process
	createFile(inoPath, string(oldSketchContent))
	logrus.Infof("restored %s", inoPath.String())

	sketchName := strings.TrimSuffix(inoPath.Base(), inoPath.Ext())
	// let's create the library corresponding to the precompiled sketch
	createLib(sketchName, buildMcu, returnJson, objFilePaths)
}

// parseCliCompileOutput function takes cmdOutToParse as argument,
// cmdOutToParse is the json output captured from the command run
// the function extracts and returns the paths of the .o files
// (generated during the compile phase) and a ReturnJson object
func parseCliCompileOutput(cmdOutToParse []byte) (*paths.PathList, *ResultJson) {
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
		logrus.Fatalf("empty directory: %s", sketchDir.String())
	}
	sketchFilesPaths.FilterSuffix(".o")

	returnJson := ResultJson{
		CoreInfo: compileOutput.BuilderResult.BuildPlatform,
		LibsInfo: compileOutput.BuilderResult.UsedLibraries,
	}

	return &sketchFilesPaths, &returnJson
}

// parseCliCompileOutputShowProp function takes cmdOutToParse as argument,
// cmdOutToParse is the output of the command run
// the function extract the value corresponding to `build.mcu` key
// that string is returned if it's found. Otherwise the program exits
func parseCliCompileOutputShowProp(cmdOutToParse []byte) string {
	cmdOut := string(cmdOutToParse)
	lines := strings.Split(cmdOut, "\n")
	for _, line := range lines {
		if strings.Contains(line, "build.mcu") { // the line should be something like: 'build.mcu=cortex-m0plus'
			if mcuLine := strings.Split(line, "="); len(mcuLine) == 2 {
				return mcuLine[1]
			}
		}
	}
	logrus.Fatal("cannot find \"build.mcu\" in arduino-cli output")
	return ""
}

// getInoSketchPath function will take argSketchPath as argument.
// and will return the path to the ino sketch
// it will run some checks along the way,
// we need the main ino file because we need to replace setup() and loop() functions in it
func getInoSketchPath(argSketchPath string) (inoPath *paths.Path) {
	sketchPath := paths.New(argSketchPath)
	if !sketchPath.Exist() {
		logrus.Fatalf("the path %s do not exist!", sketchPath.String())
	}
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
	logrus.Infof("the ino file path is %s", inoPath.String())
	return inoPath
}

// createMainCpp function, as the name suggests. will create a main.cpp file inside inoPath
// we do this because setup() and loop() functions will be replaced inside the ino file, in order to allow the linking afterwards
// creating this file is mandatory, we include also Arduino.h because it's a step done by the builder during the building phase, but only for ino files
func createMainCpp(inoPath *paths.Path) {
	// the main.cpp contains the following:
	mainCpp := `#include "Arduino.h"
void _setup();
void _loop();

void setup() {
_setup();
}

void loop() {
_loop();
}`
	mainCppPath := inoPath.Parent().Join("main.cpp")
	createFile(mainCppPath, mainCpp)
}

// removeMainCpp function, as the name suggests. will remove a main.cpp file inside inoPath
// we do this after the compile has been completed, this way we can rerun cslt-tool again.
// If we do not remove this file and run the compile again it will fail because a main.cpp file with the same definitions is already present
func removeMainCpp(inoPath *paths.Path) {
	mainCppPath := inoPath.Parent().Join("main.cpp")
	if err := os.Remove(mainCppPath.String()); err != nil {
		logrus.Warn(err)
	} else {
		logrus.Infof("removed %s", mainCppPath.String())
	}
}

// patchSketch function will modify the content of the inoPath sketch passed as argument,
// the old unmodified sketch content is returned as oldSketchContent,
// we do this to allow the compile process to succeed
func patchSketch(inoPath *paths.Path) (oldSketchContent []byte) {
	oldSketchContent, err := os.ReadFile(inoPath.String())
	if err != nil {
		logrus.Fatal(err)
	}
	if bytes.Contains(oldSketchContent, []byte("_setup()")) || bytes.Contains(oldSketchContent, []byte("_loop()")) {
		logrus.Warnf("already patched %s, skipping", inoPath.String())
	} else {
		newSketchContent := bytes.Replace(oldSketchContent, []byte("void setup()"), []byte("void _setup()"), -1)
		newSketchContent = bytes.Replace(newSketchContent, []byte("void loop()"), []byte("void _loop()"), -1)
		if err = os.WriteFile(inoPath.String(), newSketchContent, 0644); err != nil {
			logrus.Fatal(err)
		}
		logrus.Infof("replaced setup() and loop() functions in %s", inoPath.String())
	}
	return oldSketchContent
}

// createLib function will take care of creating the library directory structure and files required, for the precompiled library to be recognized as such.
// sketchName is the name of the sketch without the .ino extension. We use this for the name of the lib.
// buildMcu is the name of the MCU of the board we have compiled for. The library specifications (https://arduino.github.io/arduino-cli/0.20/library-specification/#precompiled-binaries) requires that the precompiled archive is stored inside a folder with the name of the MCU used during the compile.
// returnJson is the ResultJson object containing informations regarding core and libraries used during the compile process.
// objFilePaths is a paths.PathList containing the paths.Paths to all the sketch related object files produced during the compile phase.
func createLib(sketchName string, buildMcu string, returnJson *ResultJson, objFilePaths *paths.PathList) {
	// we are going to leverage the precompiled library infrastructure to make the linking work.
	// this type of lib, as the type suggest, is already compiled so it only gets linked during the linking phase of a sketch
	// but we have to create a library folder structure in the current directory:

	// libsketch/
	// ├── examples
	// │   └── sketch
	// │       └── sketch.ino  <-- the actual sketch we are going to compile with the arduino-cli later
	// ├── extra
	// │   └── result.json
	// ├── library.properties
	// └── src
	//     ├── cortex-m0plus
	//     │   └── libsketch.a
	//     └── libsketch.h

	// let's create the dir structure
	workingDir, err := paths.Getwd()
	if err != nil {
		logrus.Fatal(err)
	}
	libDir := workingDir.Join("lib" + sketchName)
	if libDir.Exist() { // if the dir already exixst we clean it before
		os.RemoveAll(libDir.String())
		logrus.Warnf("removed %s", libDir.String())
	}
	if err = libDir.Mkdir(); err != nil {
		logrus.Fatal(err)
	}
	srcDir := libDir.Join("src").Join(buildMcu)
	if err = srcDir.MkdirAll(); err != nil {
		logrus.Fatal(err)
	}
	exampleDir := libDir.Join("examples").Join(sketchName)
	if err = exampleDir.MkdirAll(); err != nil {
		logrus.Fatal(err)
	}
	extraDir := libDir.Join("extra")
	if err = extraDir.Mkdir(); err != nil {
		logrus.Fatal(err)
	}

	// let's create the files

	// create a library.properties file in the root dir of the lib
	// the library.properties contains the following:
	libraryProperties := `name=` + sketchName + `
sentence=This tecnically is not a library but a precompiled sketch. The result is produced using ` + os.Args[0] + `
url=https://github.com/arduino/cslt-tool
version=1.0
precompiled=true`

	libraryPropertyPath := libDir.Join("library.properties")
	createFile(libraryPropertyPath, libraryProperties)

	// we calculate the #include part to append at the beginning of the header file here with all the libraries used by the original sketch
	var librariesIncludes []string
	for _, lib := range returnJson.LibsInfo {
		for _, include := range lib.ProvidesIncludes {
			librariesIncludes = append(librariesIncludes, "#include \""+include+"\"")
		}
	}

	// create the header file in the src/ dir
	// This file has predeclarations of _setup() and _loop() functions declared originally in the main.cpp file (which is not included in the .a archive),
	// It is the counterpart of libsketch.a
	// the libsketch.h contains the following:
	libsketchHeader := strings.Join(librariesIncludes, "\n") + `
void _setup();
void _loop();`

	libsketchFilePath := srcDir.Parent().Join("lib" + sketchName + ".h")
	createFile(libsketchFilePath, libsketchHeader)

	// create the sketch file in the example dir of the lib
	// This one will include the libsketch.h and basically is the replacement of main.cpp
	// the sketch.ino contains the following:
	sketchFile := `#include <` + "lib" + sketchName + `.h>
void setup() {
	_setup();
}
void loop() {
	_loop();
}`
	sketchFilePath := exampleDir.Join(sketchName + ".ino")
	createFile(sketchFilePath, sketchFile)

	// run gcc-ar to create an archive containing all the object files except the main.cpp.o (we don't need it because we have created a substitute of it before ⬆️)
	// we exclude the main.cpp.o because we are going to link the archive libsketch.a against sketchName.ino
	objFilePaths.FilterOutPrefix("main.cpp")
	archivePath := srcDir.Join("lib" + sketchName + ".a")
	cmdArgs := append([]string{"rcs", archivePath.String()}, objFilePaths.AsStrings()...)
	logrus.Infof("running: gcc-ar %s", strings.Join(cmdArgs, " "))
	cmdOutput, err := exec.Command("gcc-ar", cmdArgs...).CombinedOutput()
	if err != nil {
		logrus.Fatal(err)
	}
	if len(cmdOutput) != 0 {
		logrus.Info(string(cmdOutput))
	} else {
		logrus.Infof("created %s", archivePath.String())
	}
	// save the result.json in the library extra dir
	jsonFilePath := extraDir.Join("result.json")
	if jsonContents, err := json.MarshalIndent(returnJson, "", " "); err != nil {
		logrus.Errorf("error serializing json: %s", err)
	} else if err := jsonFilePath.WriteFile(jsonContents); err != nil {
		logrus.Errorf("error writing %s: %s", jsonFilePath.Base(), err)
	} else {
		logrus.Infof("created %s", jsonFilePath.String())
	}
}

// createFile is an helper function useful to create a file,
// it takes filePath and fileContent as arguments,
// filePath points to the location where to save the file
// fileContent,as the name suggests, include the content of the file
func createFile(filePath *paths.Path, fileContent string) {
	err := os.WriteFile(filePath.String(), []byte(fileContent), 0644)
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.Infof("created %s", filePath.String())
}
