// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package databind

import (
	"testing"
	"time"

	"github.com/newrelic/infrastructure-agent/pkg/databind/internal/discovery"
	"github.com/newrelic/infrastructure-agent/pkg/databind/pkg/data"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextCache(t *testing.T) {
	// setup fake code
	now := time.Now()
	clock := func() time.Time {
		return now
	}
	value := "hello"
	minFetch := func() ([]discovery.Discovery, error) {
		disc := NewDiscovery(data.Map{"minute": value}, nil, nil)
		return []discovery.Discovery{disc}, nil
	}
	hourFetch := func() (interface{}, error) { return map[string]string{"value": value}, nil }
	type fetched struct{ Minute, Hour, Hour5 string }

	// GIVEN a context fetcher with cache configurations for 1 minute, 1 hour and 5 hours
	ctx := Sources{
		clock: clock,
		discoverer: &discoverer{
			cache: cachedEntry{ttl: time.Minute},
			fetch: minFetch,
		},
		variables: map[string]*gatherer{
			"hour": {
				cache: cachedEntry{ttl: time.Hour},
				fetch: hourFetch,
			},
			"hour5": {
				cache: cachedEntry{ttl: 5 * time.Hour},
				fetch: hourFetch,
			},
		},
	}

	fetch := func() fetched {
		b := New()
		vals, err := b.Fetch(&ctx)
		require.NoError(t, err)
		matches, err := b.Replace(&vals, fetched{"${minute}", "${hour.value}", "${hour5.value}"})
		require.NoError(t, err)

		require.Len(t, matches, 1)
		require.IsType(t, fetched{}, matches[0].Variables)
		return matches[0].Variables.(fetched)
	}
	// WHEN the data is fetched for the first time
	result := fetch()
	// THEN all the values are updated
	assert.Equal(t, fetched{"hello", "hello", "hello"}, result)

	// AND when the data is fetched again after the ttls expire
	value = "newValue"
	now = now.Add(5 * time.Second)
	result = fetch()
	// THEN no values are updated
	assert.Equal(t, fetched{"hello", "hello", "hello"}, result)

	// AND when the 1-minute ttl expires
	now = now.Add(60 * time.Second)
	// THEN the minute value is updated
	result = fetch()
	assert.Equal(t, fetched{"newValue", "hello", "hello"}, result)

	// AND when the 1-hour ttl expires
	now = now.Add(time.Hour)
	value = "anotherValue"
	// THEN the 1-hour (and minute) value is updated
	result = fetch()
	assert.Equal(t, fetched{"anotherValue", "anotherValue", "hello"}, result)

	// AND when the 5-hour ttl expires
	now = now.Add(5 * time.Hour)
	value = "bye"
	// THEN the all the values are updated
	result = fetch()
	assert.Equal(t, fetched{"bye", "bye", "bye"}, result)

	// AND if data is queried immediately after
	now = now.Add(5 * time.Second)
	value = "this won't be fetched!"
	// THEN no values have expired and not updated
	result = fetch()
	assert.Equal(t, fetched{"bye", "bye", "bye"}, result)
}

func mockGatherer(ttl time.Duration, data interface{}) *gatherer {
	return &gatherer{
		cache: cachedEntry{ttl: ttl},
		fetch: func() (interface{}, error) {
			return data, nil
		},
	}
}

func Test_GathererCacheTtlFromPayload(t *testing.T) {
	testCases := []struct {
		name               string
		cacheInitialTtl    time.Duration
		mockData           interface{}
		expectedTtlInCache time.Duration
	}{
		{
			name:            "no ttl implementation should respect original ttl",
			cacheInitialTtl: time.Second * 35,
			mockData: map[string]interface{}{
				"ttl": "12s",
			},
			expectedTtlInCache: time.Second * 35,
		},
		{
			name:            "ttl implementation should override original ttl",
			cacheInitialTtl: time.Second * 35,
			mockData: data.InterfaceMapWithTtl{
				"ttl": "12s",
			},
			expectedTtlInCache: time.Second * 12,
		},
		{
			name:            "ttl wrong implementation should respect original ttl",
			cacheInitialTtl: time.Second * 35,
			mockData: data.InterfaceMapWithTtl{
				"ttl": "invalid duration",
			},
			expectedTtlInCache: time.Second * 35,
		},
		{
			name:               "ttl implementation with no ttl should respect original ttl",
			cacheInitialTtl:    time.Second * 35,
			mockData:           data.InterfaceMapWithTtl{},
			expectedTtlInCache: time.Second * 35,
		},
	}

	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.name, func(t *testing.T) {
			gat := mockGatherer(testCase.cacheInitialTtl, testCase.mockData)
			source := Sources{
				clock: time.Now,
				variables: map[string]*gatherer{
					"test": gat,
				},
			}
			_, err := Fetch(&source)
			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedTtlInCache, gat.cache.ttl)
		})
	}
}

func TestTtlE2E(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		description   string
		yaml          string
		fetch         func() (interface{}, error)
		expectedTtl   time.Duration
		expectedKey   string
		expectedValue string
	}{
		{
			description: "no TTL defaults to defaultVariablesTTL",
			yaml: `
variables:
  myData:
    test:
      this: is not used in the test
`,
			expectedTtl:   defaultVariablesTTL,
			expectedKey:   "myData.data",
			expectedValue: "some_value",
			fetch: func() (interface{}, error) {
				return map[string]string{
					"data": "some_value",
				}, nil
			},
		},
		{
			description: "TTL in conf overrides defaults",
			yaml: `
variables:
  myData:
    ttl: 345s
    test:
      this: is not used in the test
`,
			expectedTtl:   time.Second * 345,
			expectedKey:   "myData.data",
			expectedValue: "some_value",
			fetch: func() (interface{}, error) {
				return map[string]string{
					"data": "some_value",
				}, nil
			},
		},
		{
			description: "TTL with no implementation has no efect in ttl",
			yaml: `
variables:
  myData:
    test:
      this: is not used in the test
`,
			expectedTtl:   defaultVariablesTTL,
			expectedKey:   "myData.data",
			expectedValue: "some_value",
			fetch: func() (interface{}, error) {
				return map[string]string{
					"data": "some_value",
					"ttl":  "1432s",
				}, nil
			},
		},
		{
			description: "TTL with implementation overrides default ttl",
			yaml: `
variables:
  myData:
    test:
      this: is not used in the test
`,
			expectedTtl:   time.Second * 1432,
			expectedKey:   "myData.data",
			expectedValue: "some_value",
			fetch: func() (interface{}, error) {
				return data.InterfaceMapWithTtl{
					"data": "some_value",
					"ttl":  "1432s",
				}, nil
			},
		},
		{
			description: "TTL with implementation overrides conf ttl",
			yaml: `
variables:
  myData:
    ttl: 345s
    test:
      this: is not used in the test
`,
			expectedTtl:   time.Second * 1432,
			expectedKey:   "myData.data",
			expectedValue: "some_value",
			fetch: func() (interface{}, error) {
				return data.InterfaceMapWithTtl{
					"data": "some_value",
					"ttl":  "1432s",
				}, nil
			},
		},
	}
	for i := range testCases {
		testCase := testCases[i]
		t.Run(testCase.description, func(t *testing.T) {
			sources, err := LoadYAML([]byte(testCase.yaml))
			assert.NoError(t, err)
			sources.clock = time.Now
			sources.variables["myData"].fetch = testCase.fetch

			values, err := Fetch(sources)
			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedValue, values.vars[testCase.expectedKey])
			assert.Equal(t, testCase.expectedTtl, sources.variables["myData"].cache.ttl)
		})
	}
}
