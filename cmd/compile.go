/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/arduino/go-paths-helper"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	semver "go.bug.st/relaxed-semver"
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
	Short: "Compiles Arduino sketches producing a precompiled library.",
	Long: `Compiles Arduino sketches producing a precompiled library:
	sketch-dist/
	├── libsketch
	│   ├── extras
	│   │   └── result.json
	│   ├── library.properties
	│   └── src
	│       ├── cortex-m0plus
	│       │   └── libsketch.a
	│       └── libsketch.h
	├── README.md  <--contains information regarding libraries and core to install in order to reproduce the original build environment
	└── sketch
	    └── sketch.ino  <-- the actual sketch we can recompile with the arduino-cli later`,
	Example: os.Args[0] + ` compile -b arduino:samd:mkrwifi1010 sketch/sketch.ino`,
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
	currentCliVersion := fmt.Sprint(unmarshalledOutput["VersionString"])
	checkCliVersion(currentCliVersion)

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

	// restore the sketch content, this allows us to rerun arduino-cslt if we want without breaking the compile process
	createFile(inoPath, string(oldSketchContent))
	logrus.Infof("restored %s", inoPath.String())

	sketchName := strings.TrimSuffix(inoPath.Base(), inoPath.Ext())
	// let's create the library corresponding to the precompiled sketch
	createLib(sketchName, buildMcu, fqbn, returnJson, objFilePaths)
}

// checkCliVersion will check if the version of the arduino-cli used is the correct one.
// It will skip the check if the version comes from a non stable release
// The version must be > 0.20.2
func checkCliVersion(currentCliVersion string) {
	logrus.Infof("arduino-cli version: %s", currentCliVersion)
	version, err := semver.Parse(currentCliVersion)
	if err == nil {
		// do the check
		incompatibleVersion, _ := semver.Parse("0.20.2")
		if version.LessThanOrEqual(incompatibleVersion) {
			logrus.Fatalf("please use a version > %s of the arduino-cli, installed version: %s", incompatibleVersion, version)
		}
	} // we continue the execution, it means that the version could be one of:
	// - git-snapshot - local build using task build
	// - nightly-<date> - nightly build
	// - test-<hash>-git-snapshot - tester builds generated by the CI system
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
			logrus.Fatalf("the sketch path specified contains multiple .ino files:\n%s\nIn order to make the magic please use the path of the .ino file containing the setup() and loop() functions", strings.Join(files.AsStrings(), "\n"))
		}
		inoPath = files[0]
	}
	logrus.Infof("the ino file path is %s", inoPath.String())
	return inoPath
}

// createMainCpp function will create a main.cpp file inside inoPath
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

// removeMainCpp function will remove a main.cpp file inside inoPath
// we do this after the compile has been completed, this way we can rerun arduino-cslt again.
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
// fqbn is required in order to generate the README.md file with instructions.
// returnJson is the ResultJson object containing informations regarding core and libraries used during the compile process.
// objFilePaths is a paths.PathList containing the paths.Paths to all the sketch related object files produced during the compile phase.
func createLib(sketchName, buildMcu, fqbn string, returnJson *ResultJson, objFilePaths *paths.PathList) {
	// we are going to leverage the precompiled library infrastructure to make the linking work.
	// this type of lib, as the type suggest, is already compiled so it only gets linked during the linking phase of a sketch
	// but we have to create a library folder structure in the current directory:

	// sketch-dist/
	// ├── libsketch
	// │   ├── extras
	// │   │   └── result.json
	// │   ├── library.properties
	// │   └── src
	// │       ├── cortex-m0plus
	// │       │   └── libsketch.a
	// │       └── libsketch.h
	// ├── README.md  <--contains information regarding libraries and core to install in order to reproduce the original build environment
	// └── sketch
	//     └── sketch.ino  <-- the actual sketch we are going to compile with the arduino-cli later

	// let's create the dir structure
	workingDir, err := paths.Getwd()
	if err != nil {
		logrus.Fatal(err)
	}
	rootDir := workingDir.Join("sketch-dist")
	if rootDir.Exist() { // if the dir already exixst we clean it before
		if err = rootDir.RemoveAll(); err != nil {
			logrus.Fatalf("cannot remove %s: %s", rootDir.String(), err)
		}
		logrus.Warnf("removed %s", rootDir.String())
	}
	if err = rootDir.Mkdir(); err != nil {
		logrus.Fatal(err)
	}
	libDir := rootDir.Join("lib" + sketchName)
	if err = libDir.Mkdir(); err != nil {
		logrus.Fatal(err)
	}
	srcDir := libDir.Join("src").Join(buildMcu)
	if err = srcDir.MkdirAll(); err != nil {
		logrus.Fatal(err)
	}
	sketchDir := rootDir.Join(sketchName)
	if err = sketchDir.MkdirAll(); err != nil {
		logrus.Fatal(err)
	}
	extraDir := libDir.Join("extras")
	if err = extraDir.Mkdir(); err != nil {
		logrus.Fatal(err)
	}

	// let's create the files

	createLibraryPropertiesFile(sketchName, libDir)

	createLibSketchHeaderFile(sketchName, srcDir, returnJson)

	sketchFilePath := createSketchFile(sketchName, sketchDir)

	createReadmeMdFile(sketchFilePath, libDir, workingDir, rootDir, returnJson)

	createArchiveFile(sketchName, objFilePaths, srcDir)

	createResultJsonFile(extraDir, returnJson)
}

// createLibraryPropertiesFile will create a library.properties file in the libDir,
// the sketchName is required in order to correctly set the name of the "library"
func createLibraryPropertiesFile(sketchName string, libDir *paths.Path) {
	// the library.properties contains the following:
	libraryProperties := `name=` + sketchName + `
author=TODO
maintainer=TODO
sentence=This technically is not a library but a precompiled sketch. The result is produced using ` + os.Args[0] + `
paragraph=
url=https://github.com/arduino/arduino-cslt
version=1.0.0
precompiled=true`

	libraryPropertyPath := libDir.Join("library.properties")
	createFile(libraryPropertyPath, libraryProperties)

}

// createLibSketchHeaderFile will create the libsketch header file,
// the file will be created in the srcDir
// This file has predeclarations of _setup() and _loop() functions declared originally in the main.cpp file (which is not included in the .a archive),
// It is the counterpart of libsketch.a
// we pass resultJson because from there we can extract infos regarding used libs
// sketchName is used to name the file
func createLibSketchHeaderFile(sketchName string, srcDir *paths.Path, returnJson *ResultJson) {
	// we calculate the #include part to append at the beginning of the header file here with all the libraries used by the original sketch
	var librariesIncludes []string
	for _, lib := range returnJson.LibsInfo {
		for _, include := range lib.ProvidesIncludes {
			librariesIncludes = append(librariesIncludes, "#include \""+include+"\"")
		}
	}

	// the libsketch.h contains the following:
	libsketchHeader := strings.Join(librariesIncludes, "\n") + `
void _setup();
void _loop();`

	libsketchFilePath := srcDir.Parent().Join("lib" + sketchName + ".h")
	createFile(libsketchFilePath, libsketchHeader)
}

// createSketchFile will create the sketch which will be the entrypoint of the compilation with the arduino-cli
// the sketch file will be created in the sketchDir
// the sketchName argument is used to correctly include the right .h file
func createSketchFile(sketchName string, sketchDir *paths.Path) *paths.Path {
	// This one will include the libsketch.h and basically is the replacement of main.cpp
	// the sketch.ino contains the following:
	sketchFile := `#include <` + "lib" + sketchName + `.h>
void setup() {
  _setup();
}
void loop() {
  _loop();
}`
	sketchFilePath := sketchDir.Join(sketchName + ".ino")
	createFile(sketchFilePath, sketchFile)
	return sketchFilePath
}

// createReadmeMdFile is a helper function that is reposnible for the generation of the README.md file containing informations on how to reproduce the build environment
// it takes the resultJson and some paths.Paths as input to do the required calculations.. The name of the arguments should be sufficient to understand
func createReadmeMdFile(sketchFilePath, libDir, workingDir, rootDir *paths.Path, returnJson *ResultJson) {
	// generate the commands to run to successfully reproduce the build environment, they will be used as content for the README.md
	var readmeContent []string
	readmeContent = append(readmeContent, "`arduino-cli core install "+returnJson.CoreInfo.Id+"@"+returnJson.CoreInfo.Version+"`")
	libs := []string{}
	for _, l := range returnJson.LibsInfo {
		libs = append(libs, l.Name+"@"+l.Version)
	}
	readmeContent = append(readmeContent, fmt.Sprintf("`arduino-cli lib install %s`", strings.Join(libs, " ")))
	// make the paths relative, absolute paths are too long and are different on the user machine
	sketchFileRelPath, _ := sketchFilePath.RelFrom(workingDir)
	libRelDir, _ := libDir.RelFrom(workingDir)
	readmeCompile := "`arduino-cli compile -b " + fqbn + " " + sketchFileRelPath.String() + " --library " + libRelDir.String() + "`"

	//create the README.md file containig instructions regarding what commands to run in order to have again a working binary
	// the README.md contains the following:
	readmeMd := `This package contains firmware code loaded in your product. 
The firmware contains additional code licensed with LGPL clause; in order to re-compile the entire firmware bundle, please execute the following.

## Install core and libraries
` + strings.Join(readmeContent, "\n") + "\n" + `
## Compile
` + readmeCompile + "\n"

	readmeMdPath := rootDir.Join("README.md")
	createFile(readmeMdPath, readmeMd)
}

// createArchiveFile function will run `gcc-ar` to create an archive containing all the object files except the main.cpp.o (we don't need it because we have created a substitute of it before: sketchfile.ino)
func createArchiveFile(sketchName string, objFilePaths *paths.PathList, srcDir *paths.Path) {
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
}

// createResultJsonFile will generate the result.json file and save it in extraDir
func createResultJsonFile(extraDir *paths.Path, returnJson *ResultJson) {
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
// fileContent include the content of the file
func createFile(filePath *paths.Path, fileContent string) {
	err := os.WriteFile(filePath.String(), []byte(fileContent), 0644)
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.Infof("created %s", filePath.String())
}
