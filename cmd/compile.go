/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"encoding/json"
	"os"
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
	BuildPath     string  `json:"build_path"`
	UsedLibraries []*Info `json:"used_libraries"`
}

// Info contains information regarding the library or the core used during the compile process
type Info struct {
	Packager string `json:"packager,omitempty"`
	Name     string `json:"name"`
	Version  string `json:"version"`
}

// returnJson contains information regarding the core and libraries used during the compile process
type ReturnJson struct {
	CoreInfo *Info   `json:"coreInfo"`
	LibsInfo []*Info `json:"libsInfo"`
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
// cmdOutToParse is the json output captured from the command run
// the function extracts and returns the paths of the .o files
// (generated during the compile phase) and a ReturnJson object
func parseOutput(cmdOutToParse []byte) ([]*paths.Path, *ReturnJson) {
	err := json.Unmarshal(cmdOutToParse, &compileOutput)
	if err != nil {
		logrus.Fatal(err)
	} else if !compileOutput.Success {
		logrus.Fatalf("sketch compile was not successful: %s", compileOutput.CompilerErr)
	}

	compilerOutLines := strings.Split(compileOutput.CompilerOut, "\n")
	var platformPath *paths.Path
	for _, compilerOutLine := range compilerOutLines {
		logrus.Debug(compilerOutLine)
		if platformPath = parsePlatformLine(compilerOutLine); platformPath != nil {
			break
		}
	}
	if platformPath == nil {
		logrus.Fatal("cannot find platform used")
	}

	// this dir contains all the obj files we need (the sketch related ones and not the core or libs)
	sketchDir := paths.New(compileOutput.BuilderResult.BuildPath).Join("sketch")
	sketchFilesPaths, err := sketchDir.ReadDir()
	if err != nil {
		logrus.Fatal(err)
	} else if len(sketchFilesPaths) == 0 {
		logrus.Fatalf("empty directory: %s", sketchDir)
	}
	var returnObjectFilesList []*paths.Path
	for _, sketchFilePath := range sketchFilesPaths {
		if sketchFilePath.Ext() == ".o" {
			returnObjectFilesList = append(returnObjectFilesList, sketchFilePath)
		}
	}

	returnJson := ReturnJson{
		CoreInfo: getCoreInfo(platformPath),
		LibsInfo: compileOutput.BuilderResult.UsedLibraries,
	}

	return returnObjectFilesList, &returnJson
}

// getCoreInfo takes the path of the platform used and
// returns an Info object
func getCoreInfo(platformPath *paths.Path) *Info {
	if !platformPath.Exist() {
		logrus.Fatalf("the path of the core does not exists: %s", platformPath)
	}
	corePathParents := platformPath.Parents()
	coreInfo := &Info{
		Packager: corePathParents[3].Base(),
		Name:     corePathParents[1].Base(),
		Version:  corePathParents[0].Base(),
	}
	return coreInfo
}

// parsePlatformLine takes compilerOutLine as input and tries to extract the path of the platform used
// compilerOutLine should be something like:
// - Using core 'arduino' from platform in folder: C:\\Users\\Umberto Baldi\\AppData\\Local\\Arduino15\\packages\\arduino\\hardware\\avr\\1.8.5\n
// - Using core 'arduino' from platform in folder: /home/umberto/.arduino15/packages/arduino/hardware/avr/1.8.4

func parsePlatformLine(compilerOutLine string) *paths.Path {
	if matched := strings.Contains(compilerOutLine, "Using core"); matched {
		// we use this approach to avoid path with spaces problem
		if startIndex := strings.Index(compilerOutLine, "from platform in folder: "); startIndex != -1 {
			startIndex = startIndex + len("from platform in folder: ")
			return paths.New(compilerOutLine[startIndex:])
		}
	}
	return nil
}
