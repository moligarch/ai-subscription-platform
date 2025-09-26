//go:build tools
// +build tools

// This file pins dev tools (like oapi-codegen) into go.mod
// so everyone/CI uses the same versions. It is excluded from
// normal builds by the 'tools' build tag above.

package tools

import (
	_ "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen"
)