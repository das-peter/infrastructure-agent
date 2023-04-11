// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package data

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	DiscoveryPrefix             = "discovery."
	LabelInfix                  = "label."
	ContainerReplaceFieldPrefix = "container:"

	Port                       = "port"
	Ports                      = "ports"
	IP                         = "ip"
	PrivatePort                = "private.port"
	PrivatePorts               = "private.ports"
	PrivateIP                  = "private.ip"
	Name                       = "name"
	Image                      = "image"
	ImageID                    = "imageId"
	ContainerID                = "containerId"
	ContainerName              = "containerName"
	Label                      = "label"
	Command                    = "command"
	DockerContainerName        = "dockerContainerName"
	EntityRewriteActionReplace = "replace"
)

var ErrNotFound = errors.New("TTL value not found")

type ValuesWithTtl interface {
	TTL() (time.Duration, error)
}

type (
	Map          map[string]string
	InterfaceMap map[string]interface{}
	// InterfaceMapWithTtl is a custom implementation of a payload exposing a TTL
	InterfaceMapWithTtl map[string]interface{}
)

func (ttl InterfaceMapWithTtl) TTL() (time.Duration, error) {
	if val, ok := ttl["ttl"]; ok {
		return time.ParseDuration(val.(string))
	}
	return 0, ErrNotFound
}

type Transformed struct {
	Variables         interface{}
	MetricAnnotations Map
	EntityRewrites    []EntityRewrite
}

type EntityRewrite struct {
	Action       string `json:"action"`
	Match        string `json:"match"`
	ReplaceField string `json:"replaceField"`
}

type EntityRewrites []EntityRewrite

func InterfaceMapToMap(original InterfaceMap) (out Map) {
	out = make(Map, len(original))
	AddValues(out, "", original)
	return out
}

type GenericDiscovery struct {
	Variables      InterfaceMap    `json:"variables"`
	Annotations    InterfaceMap    `json:"metricAnnotations"`
	EntityRewrites []EntityRewrite `json:"entityRewrites"`
}

// Apply tries to match and replace entityName according to EntityRewrite configuration.
func (e EntityRewrites) Apply(entityName string) string {
	result := entityName

	for _, er := range e {
		if er.Action == EntityRewriteActionReplace {
			result = strings.Replace(result, er.Match, er.ReplaceField, -1)
		}
	}

	return result
}

// Adds a structured value to a flat map, where each key has a
// JS-like notation to access fields or arrays, if the value is
// structured
// e.g. if val is a map {"prop":{"arr":[1,2,3]}} it would add
// the following entries to the destination map
// "prop.arr[0]" : "1"
// "prop.arr[1]" : "2"
// "prop.arr[2]" : "3"
// please note that int values are converted to string
func AddValues(dst Map, prefix string, val interface{}) {
	var pfx string
	if prefix != "" {
		pfx = prefix + "."
	} else {
		pfx = ""
	}
	switch value := val.(type) {
	case string:
		dst[prefix] = value
	case map[string]string:
		for k, v := range value {
			dst[pfx+k] = v
		}
	case map[string]interface{}:
		for k, v := range value {
			AddValues(dst, pfx+k, v)
		}
	case InterfaceMap:
		for k, v := range value {
			AddValues(dst, pfx+k, v)
		}
	case InterfaceMapWithTtl:
		for k, v := range value {
			AddValues(dst, pfx+k, v)
		}
	case []string:
		for k, v := range value {
			dst[prefix+"["+strconv.Itoa(k)+"]"] = v
		}
	case []interface{}:
		for k, v := range value {
			AddValues(dst, prefix+"["+strconv.Itoa(k)+"]", v)
		}
	default:
		dst[prefix] = fmt.Sprintf("%+v", value)
	}
}
