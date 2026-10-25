package main

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDijkstra(t *testing.T) {
	testCases := []struct {
		name                 string
		start                string
		destination          string
		m                    map[string]map[string]int
		expectedStatus       string
		expectedPath         string
		expectedPathDistance string
		expectedDistance     int
	}{
		{
			name:        "reachable - c to d looks good but c to e is better",
			start:       "a",
			destination: "e",
			m: map[string]map[string]int{
				"a": {"b": 2, "c": 7},
				"b": {"c": 3, "d": 8},
				"c": {"d": 1, "e": 6},
				"d": {"e": 20},
				"e": {},
			},
			expectedStatus:       "reachable",
			expectedPath:         "a -> b -> c -> e",
			expectedPathDistance: "0 -> 2 -> 5 -> 11",
			expectedDistance:     11,
		},
		{
			name:        "reachable - each path visited",
			start:       "a",
			destination: "e",
			m: map[string]map[string]int{
				"a": {"b": 2, "c": 7},
				"b": {"c": 3, "d": 8},
				"c": {"d": 1, "e": 6},
				"d": {"e": 2},
				"e": {},
			},
			expectedStatus:       "reachable",
			expectedPath:         "a -> b -> c -> d -> e",
			expectedPathDistance: "0 -> 2 -> 5 -> 6 -> 8",
			expectedDistance:     8,
		},
		{
			name:        "reachable - start and destination are the same",
			start:       "a",
			destination: "a",
			m: map[string]map[string]int{
				"a": {"b": 2, "c": 7},
				"b": {"c": 3, "d": 8},
				"c": {"d": 1, "e": 6},
				"d": {"e": 2},
				"e": {},
			},
			expectedStatus:       "reachable",
			expectedPath:         "a",
			expectedPathDistance: "0",
			expectedDistance:     0,
		},
		{
			name:        "unreachable - no way to go to e",
			start:       "a",
			destination: "e",
			m: map[string]map[string]int{
				"a": {"b": 2, "c": 7},
				"b": {"c": 3, "d": 8},
				"c": {"d": 1},
				"d": {},
				"e": {},
			},
			expectedStatus:       "unreachable",
			expectedPath:         "",
			expectedPathDistance: "",
			expectedDistance:     0,
		},
		{
			name:        "reachable - when there is a dead end",
			start:       "a",
			destination: "e",
			m: map[string]map[string]int{
				"a": {"b": 2, "c": 7, "f": 1},
				"b": {"c": 3, "d": 8},
				"c": {"d": 1, "e": 6},
				"d": {"e": 2},
				"e": {},
				"f": nil,
			},
			expectedStatus:       "reachable",
			expectedPath:         "a -> b -> c -> d -> e",
			expectedPathDistance: "0 -> 2 -> 5 -> 6 -> 8",
			expectedDistance:     8,
		},
		{
			name:        "unreachable - destination unknown",
			start:       "a",
			destination: "z",
			m: map[string]map[string]int{
				"a": {"b": 2, "c": 7},
				"b": {"c": 3, "d": 8},
				"c": {"d": 1, "e": 6},
				"d": {"e": 20},
				"e": {},
			},
			expectedStatus:       "unreachable",
			expectedPath:         "",
			expectedPathDistance: "",
			expectedDistance:     0,
		},
		{
			name:        "unreachable - start is unknown",
			start:       "z",
			destination: "e",
			m: map[string]map[string]int{
				"a": {"b": 2, "c": 7},
				"b": {"c": 3, "d": 8},
				"c": {"d": 1, "e": 6},
				"d": {"e": 20},
				"e": {},
			},
			expectedStatus:       "unreachable",
			expectedPath:         "",
			expectedPathDistance: "",
			expectedDistance:     0,
		},
		{
			name:        "unreachable - start and destination are unknown",
			start:       "z",
			destination: "z",
			m: map[string]map[string]int{
				"a": {"b": 2, "c": 7},
				"b": {"c": 3, "d": 8},
				"c": {"d": 1, "e": 6},
				"d": {"e": 20},
				"e": {},
			},
			expectedStatus:       "unreachable",
			expectedPath:         "",
			expectedPathDistance: "",
			expectedDistance:     0,
		},
		{
			name:        "unreachable - only island g leads to e",
			start:       "a",
			destination: "e",
			m: map[string]map[string]int{
				"a": {"b": 2, "c": 7},
				"b": {"c": 3, "d": 8},
				"c": {"d": 1},
				"d": {},
				"e": {},
				"g": {"e": 1},
			},
			expectedStatus:       "unreachable",
			expectedPath:         "",
			expectedPathDistance: "",
			expectedDistance:     0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			note, route := Dijkstra(tc.start, tc.destination, tc.m)

			status, path, pathDistance, distance := Path(tc.destination, note, route)

			assert.Equal(t, tc.expectedStatus, status)
			assert.Equal(t, tc.expectedPath, path)
			assert.Equal(t, tc.expectedPathDistance, pathDistance)
			assert.Equal(t, tc.expectedDistance, distance)
		})
	}

}

func TestDijkstraNote(t *testing.T) {
	testCases := []struct {
		name         string
		start        string
		destination  string
		m            map[string]map[string]int
		place        string
		expectedNote int
	}{
		{
			name:        "island g must not overflow e",
			start:       "a",
			destination: "z",
			m: map[string]map[string]int{
				"a": {"b": 2, "c": 7},
				"b": {"c": 3, "d": 8},
				"c": {"d": 1},
				"d": {},
				"e": {},
				"g": {"e": 1},
			},
			place:        "e",
			expectedNote: math.MaxInt,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			note, _ := Dijkstra(tc.start, tc.destination, tc.m)

			assert.Equal(t, tc.expectedNote, note[tc.place])
		})
	}
}
