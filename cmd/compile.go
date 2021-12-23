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
	parseOutput(cmdOutput) // TODO save the return

	// TODO:
	// copy sketch.ino.o from tmp/... in current dir or in --output dir
}

// parseOutput function takes cmdOutToParse as argument,
// cmdOutToParse is the output captured from the command run
// the function extract the path of the sketck.ino.o and
// a returnJson object
func parseOutput(cmdOutToParse []byte) (*paths.Path, *ReturnJson) {
	err := json.Unmarshal(cmdOutToParse, &compileOutput)
	if err != nil {
		logrus.Fatal(err)
	} else if !compileOutput.Success {
		logrus.Fatalf("sketch compile was not successful %s", compileOutput.CompilerErr)
	}

	compilerOutLines := strings.Split(compileOutput.CompilerOut, "\n")
	var coreLine string
	var objFilePath string
	for _, compilerOutLine := range compilerOutLines {
		logrus.Info(compilerOutLine)
		if matched := strings.Contains(compilerOutLine, "Using core"); matched {
			coreLine = compilerOutLine
		}
		if objFilePath = ParseObjFilePath(compilerOutLine); objFilePath != "" {
			break // we should already have coreLine
		}

	}
	if coreLine == "" {
		logrus.Fatal("cannot find core used")
	}
	if objFilePath == "" {
		logrus.Fatal("cannot find sketch object file")
	}

	returnJson := ReturnJson{
		CoreInfo: parseCoreLine(coreLine),
		LibsInfo: compileOutput.BuilderResult.UsedLibraries,
	}

	// TODO missing calculation of <sketch>.ino.o file
	// TODO there could be multiple `.o` files, see zube comment. The correct approach is to go in `<tempdir>/arduino-sketch_stuff/sketch` and copy all the `.o` files
	// TODO add also the packager -> maybe ParseReference could be used from the cli
	// TODO core could be calculated from fqbn

	return nil, &returnJson //TODO remove
}

func ParseObjFilePath(compilerOutLine string) (objFilePath string) {
	var endChar string
	if index := strings.Index(compilerOutLine, ".ino.cpp.o"); index != -1 {
		sketchEndIndex := index + len(".ino.cpp.o")
		if sketchEndIndex >= len(compilerOutLine) { // this means the path terminates with the `o` and thus the last character is space
			endChar = " "
		} else {
			endChar = string(compilerOutLine[sketchEndIndex])
		}
		// TODO see conversation with silvano: if endchar is preceded by / proceed in the search
		sketchStartIndex := strings.LastIndex(compilerOutLine[:sketchEndIndex], endChar) + 1
		objFilePath = compilerOutLine[sketchStartIndex:sketchEndIndex]
		return objFilePath
	} else {
		return ""
	}
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
