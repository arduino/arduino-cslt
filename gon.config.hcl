
# Source: https://github.com/arduino/tooling-project-assets/blob/main/workflow-templates/assets/general/gon.config.hcl
# See: https://github.com/mitchellh/gon#configuration-file
source = ["dist/arduino-cslt_osx_darwin_amd64/arduino-cslt"]
bundle_id = "cc.arduino.arduino-cslt"

sign {
  application_identity = "Developer ID Application: ARDUINO SA (7KT7ZWMCJT)"
}

# Ask Gon for zip output to force notarization process to take place.
# The CI will ignore the zip output, using the signed binary only.
zip {
  output_path = "unused.zip"
}