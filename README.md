# Struct to Docs

A mini app designed to automatically generate documentation from Go code. This app uses ast to crawl through a file system path generating a knowledge map of structs and then outputs to stdout including information about embedded / linked structs

**NOTE** This project was very low priority so, after initial implementation, AI was used to program most of the functionality with little attention paid to quality. I apologize for any odd details but, welcome improvements!

## Options

Struct to Docs has some flags available to control and limit output

* **dirPath** - Root directory to Directory to starting from
* **struct** - Either exact name or pattern for structs to include. Supports * and ?
* **dirFilter** - Directory inclusion path filter. Supports * and ?
* **allowCreateMod** - In the rare event building docs for a project that doesn't have a `go.mod` file, create a temporary one. Primarily intended for tests

## Example

### Input

```
// ToolRef A human readable but, machine safe identifier for the tool
type ToolRef string

// Tool Base tool record
type Tool struct {
	// Name Human readable title representing the tool name
	Name string `json:"name" yaml:"name"`
	// Ref A human readable but, machine safe identifier for the tool
	Ref ToolRef `json:"ref" yaml:"ref"`
	// Summary a short description highlighting the purpose of the tool
	Summary string `json:"summary" yaml:"summary"`
	// Description Long form description of the tool
	Description string `json:"description" yaml:"description"`
	// HomePage URL pointing to the official home page of the tool
	HomePage string `json:"home_page" yaml:"home_page"`
	// IconURL Custom URL pointing to an image that can be used where icons presented to the user
	IconURL string `json:"icon_url,omitempty" yaml:"icon_url"`
	// License URL pointing to the tool license agreement
	License string `json:"license" yaml:"license"`
	// Dependencies A slice of tool references that need to also be available before tool installation / use
	Dependencies []ToolRef `json:"dependencies,omitempty" yaml:"dependencies"`
	// Binaries A slice of path references to executable files that should be made available to the environments
	Binaries []string `json:"binaries" yaml:"binaries"`
	// Environment System environment variable declarations to be included at operating time
	Environment []string `json:"environment" yaml:"environment"`
	// Tags A slice of custom taxonomy that can be used to characterise the tool
	Tags []string `json:"tags" yaml:"tags"`
	// Installs collection of install records letting the system know what is available for reuse
	Installs []*ToolInstall `json:"installs,omitempty" yaml:"installs"`
}

type ToolInstall struct {
	// ToolRef A human readable but, machine safe identifier for the tool
	ToolRef string `json:"tool_ref" yaml:"tool_ref"`
	// Version Semantic version string
	Version string `json:"version" yaml:"version"`
	// Platform What operating system this release record is relevant to
	Platform string `json:"platform" yaml:"platform"`
}
```

### Output

```
# Name Human readable title representing the tool name
name: <string>
# Ref A human readable but, machine safe identifier for the tool
ref: <ToolRef>
# Summary a short description highlighting the purpose of the tool
summary: <string>
# Description Long form description of the tool
description: <string>
# HomePage URL pointing to the official home page of the tool
home_page: <string>
# IconURL Custom URL pointing to an image that can be used where icons presented to the user
icon_url: <string>
# License URL pointing to the tool license agreement
license: <string>
# Dependencies A slice of tool references that need to also be available before tool installation / use
dependencies: <[]ToolRef>
# Binaries A slice of path references to executable files that should be made available to the environments
binaries: <[]string>
# Environment System environment variable declarations to be included at operating time
environment: <[]string>
# Tags A slice of custom taxonomy that can be used to characterise the tool
tags: <[]string>
# Installs collection of install records letting the system know what is available for reuse
installs: <[]*ToolInstall>
-
  # ToolRef A human readable but, machine safe identifier for the tool
  tool_ref: <string>
  # Version Semantic version string
  version: <string>
  # Platform What operating system this release record is relevant to
  platform: <string>
```