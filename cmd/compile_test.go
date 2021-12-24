package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var goodPath = []struct {
	cliOut string
	path   string
}{
	{`/home/umberto/.arduino15/packages/arduino/tools/arm-none-eabi-gcc/7-2017q4/bin/arm-none-eabi-g++ -mcpu=cortex-m0plus -mthumb -c -g -Os -w -std=gnu++11 -ffunction-sections -fdata-sections -fno-threadsafe-statics -nostdlib --param max-inline-insns-single=500 -fno-rtti -fno-exceptions -MMD -DF_CPU=48000000L -DARDUINO=10607 -DARDUINO_SAMD_MKRWAN1310 -DARDUINO_ARCH_SAMD -DUSE_ARDUINO_MKR_PIN_LAYOUT -D__SAMD21G18A__ -DUSB_VID=0x2341 -DUSB_PID=0x8059 -DUSBCON "-DUSB_MANUFACTURER=\"Arduino LLC\"" "-DUSB_PRODUCT=\"Arduino MKR WAN 1310\"" -DUSE_BQ24195L_PMIC -DVERY_LOW_POWER -I/home/umberto/.arduino15/packages/arduino/tools/CMSIS/4.5.0/CMSIS/Include/ -I/home/umberto/.arduino15/packages/arduino/tools/CMSIS-Atmel/1.2.0/CMSIS/Device/ATMEL/ -I/home/umberto/.arduino15/packages/arduino/hardware/samd/1.8.12/cores/arduino/api/deprecated -I/home/umberto/.arduino15/packages/arduino/hardware/samd/1.8.12/cores/arduino/api/deprecated-avr-comp -I/home/umberto/.arduino15/packages/arduino/hardware/samd/1.8.12/cores/arduino -I/home/umberto/.arduino15/packages/arduino/hardware/samd/1.8.12/variants/mkrwan1300 -I/home/umberto/Arduino/libraries/MKRWAN/src /tmp/arduino-sketch-6631A0D1FB86504F68374499C713AA5D/sketch/getdeveui.ino.cpp -o /tmp/arduino-sketch-6631A0D1FB86504F68374499C713AA5D/sketch/getdeveui.ino.cpp.o`, `/tmp/arduino-sketch-6631A0D1FB86504F68374499C713AA5D/sketch/getdeveui.ino.cpp.o`},
	{`/home/umberto/.arduino15/packages/arduino/tools/arm-none-eabi-gcc/7-2017q4/bin/arm-none-eabi-g++ -mcpu=cortex-m0plus -mthumb -c -g -Os -w -std=gnu++11 -ffunction-sections -fdata-sections -fno-threadsafe-statics -nostdlib --param max-inline-insns-single=500 -fno-rtti -fno-exceptions -MMD -DF_CPU=48000000L -DARDUINO=10607 -DARDUINO_SAMD_MKRWAN1310 -DARDUINO_ARCH_SAMD -DUSE_ARDUINO_MKR_PIN_LAYOUT -D__SAMD21G18A__ -DUSB_VID=0x2341 -DUSB_PID=0x8059 -DUSBCON "-DUSB_MANUFACTURER=\"Arduino LLC\"" "-DUSB_PRODUCT=\"Arduino MKR WAN 1310\"" -DUSE_BQ24195L_PMIC -DVERY_LOW_POWER -I/home/umberto/.arduino15/packages/arduino/tools/CMSIS/4.5.0/CMSIS/Include/ -I/home/umberto/.arduino15/packages/arduino/tools/CMSIS-Atmel/1.2.0/CMSIS/Device/ATMEL/ -I/home/umberto/.arduino15/packages/arduino/hardware/samd/1.8.12/cores/arduino/api/deprecated -I/home/umberto/.arduino15/packages/arduino/hardware/samd/1.8.12/cores/arduino/api/deprecated-avr-comp -I/home/umberto/.arduino15/packages/arduino/hardware/samd/1.8.12/cores/arduino -I/home/umberto/.arduino15/packages/arduino/hardware/samd/1.8.12/variants/mkrwan1300 -I/home/umberto/Arduino/libraries/MKRWAN/src "/tmp/arduino-sketch-184EE89862BF2EE5FF75311EE55FC1DD/sketch/getdeveui (copy).ino.cpp" -o "/tmp/arduino-sketch-184EE89862BF2EE5FF75311EE55FC1DD/sketch/getdeveui (copy).ino.cpp.o"`, `/tmp/arduino-sketch-184EE89862BF2EE5FF75311EE55FC1DD/sketch/getdeveui (copy).ino.cpp.o`},
	{`/home/umberto/.arduino15/packages/arduino/tools/arm-none-eabi-gcc/7-2017q4/bin/arm-none-eabi-g++ -mcpu=cortex-m0plus -mthumb -c -g -Os -w -std=gnu++11 -ffunction-sections -fdata-sections -fno-threadsafe-statics -nostdlib --param max-inline-insns-single=500 -fno-rtti -fno-exceptions -MMD -DF_CPU=48000000L -DARDUINO=10607 -DARDUINO_SAMD_MKRWAN1310 -DARDUINO_ARCH_SAMD -DUSE_ARDUINO_MKR_PIN_LAYOUT -D__SAMD21G18A__ -DUSB_VID=0x2341 -DUSB_PID=0x8059 -DUSBCON "-DUSB_MANUFACTURER=\"Arduino LLC\"" "-DUSB_PRODUCT=\"Arduino MKR WAN 1310\"" -DUSE_BQ24195L_PMIC -DVERY_LOW_POWER -I/home/umberto/.arduino15/packages/arduino/tools/CMSIS/4.5.0/CMSIS/Include/ -I/home/umberto/.arduino15/packages/arduino/tools/CMSIS-Atmel/1.2.0/CMSIS/Device/ATMEL/ -I/home/umberto/.arduino15/packages/arduino/hardware/samd/1.8.12/cores/arduino/api/deprecated -I/home/umberto/.arduino15/packages/arduino/hardware/samd/1.8.12/cores/arduino/api/deprecated-avr-comp -I/home/umberto/.arduino15/packages/arduino/hardware/samd/1.8.12/cores/arduino -I/home/umberto/.arduino15/packages/arduino/hardware/samd/1.8.12/variants/mkrwan1300 -I/home/umberto/Arduino/libraries/MKRWAN/src "/tmp/arduino-sketch-A51E030A1327FDE41CD99CC549EF531E/sketch/getdeveui (another \"copy\").ino.cpp" -o "/tmp/arduino-sketch-A51E030A1327FDE41CD99CC549EF531E/sketch/getdeveui (another \"copy\").ino.cpp.o"`, `/tmp/arduino-sketch-A51E030A1327FDE41CD99CC549EF531E/sketch/getdeveui (another \"copy\").ino.cpp.o`},
	// TODO add path on windows
}

func TestObjFilePathParse(t *testing.T) {
	for _, test := range goodPath {
		objFilePath := ParseObjFilePath(test.cliOut)
		assert.Equal(t, test.path, objFilePath)
	}

}
