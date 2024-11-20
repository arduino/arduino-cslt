# arduino-cslt

`arduino-cslt` is a convenient wrapper of [arduino-cli](https://github.com/arduino/arduino-cli), it compiles Arduino sketches outputting a precompiled library under `sketch-dist/` folder created in the current working directory.
It generates a README.md file that contains information regarding libraries and core to use in order to build the sketch. The result is achieved by parsing the verbose output of `arduino-cli` and by using [GNU ar](https://sourceware.org/binutils/docs/binutils/ar.html) to generate an archive of the object files.

## Prerequisites
In order to run this tool you have to install first the [Arduino CLI](https://github.com/arduino/arduino-cli) and have `arduino-cli` binary in your `$PATH`, otherwise `arduino-cslt` won't work.
Please use a version of the Arduino CLI that has [this](https://github.com/arduino/arduino-cli/pull/1608) change (version > 0.20.2).

Another requirement is [`gcc-ar`](https://sourceware.org/binutils/docs/binutils/ar.html) in your `$PATH`.

- On Linux `gcc-ar` can be obtained by installing GCC (e.g., `apt-get install gcc`).
- On macOS `gcc-ar` can be obtained by installing `binutils` (e.g. `brew install binutils`) and creating a symlink from `gar` to `gcc-ar` (`ln -s /usr/local/Cellar/binutils/2.39_1/bin/gar /usr/local/bin/gcc-ar`).
- On Windows `gcc-ar` can be obtained by installing `binutils` (e.g. `scoop install binutils`).

## Build it
In order to build `arduino-cslt` just use `task go:build`

## Usage
`./arduino-cslt compile -b <fqbn> <sketch_path>`

[![asciicast](https://asciinema.org/a/465059.svg)](https://asciinema.org/a/465059)

For example, running `./arduino-cslt compile -b arduino:samd:mkrwifi1010 sketch/sketch.ino` should produce a library with the following structure, in the current working directory:
```
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
    └── sketch.ino  <-- the actual sketch we are going to compile with the arduino-cli later
```

This is an example execution:
```
$ ./arduino-cslt compile -b arduino:samd:mkrwifi1010 sketch/sketch.ino
INFO[0000] arduino-cli version: git-snapshot            
INFO[0000] GNU ar (GNU Binutils) 2.37                   
INFO[0000] the ino file path is sketch/sketch.ino 
INFO[0000] created sketch/main.cpp 
INFO[0000] replaced setup() and loop() functions in sketch/sketch.ino 
INFO[0000] running: arduino-cli compile -b arduino:samd:mkrwifi1010 sketch/sketch.ino -v --format json 
INFO[0000] running: arduino-cli compile -b arduino:samd:mkrwifi1010 sketch/sketch.ino --show-properties 
INFO[0001] removed sketch/main.cpp 
INFO[0001] created sketch/sketch.ino 
INFO[0001] restored sketch/sketch.ino 
INFO[0001] created sketch-dist/libsketch/library.properties
INFO[0001] created sketch-dist/libsketch/src/libsketch.h 
INFO[0001] created sketch-dist/sketch/sketch.ino 
INFO[0003] created sketch-dist/README.md 
INFO[0001] running: gcc-ar rcs sketch-dist/libsketch/src/cortex-m0plus/libsketch.a /tmp/arduino-sketch-E4D76B1781E9EB73A7B3491CAC68F374/sketch/sketch.ino.cpp.o 
INFO[0001] created sketch-dist/libsketch/src/cortex-m0plus/libsketch.a 
INFO[0001] created sketch-dist/libsketch/extras/result.json
```

The content of `sketch-dist/README.md` included copy-pastable commands to reproduce the build environment:
```markdown
This package contains firmware code loaded in your product. 
The firmware contains additional code licensed with LGPL clause; in order to re-compile the entire firmware bundle, please execute the following.

## Install core and libraries
`arduino-cli core install arduino:samd@1.8.12`
`arduino-cli lib install WiFiNINA@1.8.13 SPI@1.0`

## Compile
`arduino-cli compile -b arduino:samd:mkrwifi1010 sketch-dist/sketch/sketch.ino --library sketch-dist/libsketch`
```

And the content of `sketch-dist/libsketch/extras/result.json` is:
```json
{
 "coreInfo": {
  "id": "arduino:samd",
  "version": "1.8.12"
 },
 "libsInfo": [
  {
   "name": "WiFiNINA",
   "version": "1.8.13",
   "provides_includes": [
    "WiFiNINA.h"
   ]
  },
  {
   "name": "SPI",
   "version": "1.0",
   "provides_includes": [
    "SPI.h"
   ]
  }
 ]
}
```

## How to compile the precompiled sketch
In order to compile the sketch you can follow the instructions listed in the `sketch-dist/README.md` file.

You can install a core with [`arduino-cli core install PACKAGER:ARCH[@VERSION]`](https://arduino.github.io/arduino-cli/latest/commands/arduino-cli_core_install/).

You can install a library with [`arduino-cli lib install LIBRARY[@VERSION_NUMBER]`](https://arduino.github.io/arduino-cli/latest/commands/arduino-cli_lib_install/).

After completing that operation you can compile it with:

`arduino-cli compile -b <fqbn> sketch-dist/sketch/sketch.ino --library sketch-dist/<libsketch>`.

It's important to use the `--library` flag to include the precompiled library generated with arduino-cslt otherwise the Arduino CLI won't find it.

For example a legit execution looks like this:
```
$ arduino-cli compile -b arduino:samd:mkrwifi1010 sketch-dist/sketch/sketch.ino --library sketch-dist/libsketch/

Library libsketch has been declared precompiled:
Using precompiled library in sketch-dist/libsketch/src/cortex-m0plus
Sketch uses 14636 bytes (5%) of program storage space. Maximum is 262144 bytes.
Global variables use 3224 bytes (9%) of dynamic memory, leaving 29544 bytes for local variables. Maximum is 32768 bytes.
```
