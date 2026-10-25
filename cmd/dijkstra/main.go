package main

import (
	"fmt"
	"math"
	"strconv"
)

func main() {
	start := "a"
	destination := "e"
	m := map[string]map[string]int{
		"a": {"b": 2, "c": 7},
		"b": {"c": 3, "d": 8},
		"c": {"d": 1, "e": 6},
		"d": {"e": 2},
		"e": {},
	}

	note, route := Dijkstra(start, destination, m)

	status, path, pathDistance, distance := Path(destination, note, route)

	fmt.Printf("%s: %v\n", "status", status)
	fmt.Printf("%s: %v\n", "path", path)
	fmt.Printf("%s: %s\n", "path distance", pathDistance)
	fmt.Printf("%s: %d\n", "distance", distance)
}

func Dijkstra(start string, destination string, fullMap map[string]map[string]int) (note map[string]int, route map[string]string) {
	visited := map[string]bool{}
	note = map[string]int{}
	route = map[string]string{}

	//for destination, minute := range m["b"] {
	//	fmt.Println("b →", destination, ":", minute)
	//}

	for place := range fullMap {
		note[place] = math.MaxInt
	}

	if _, ok := fullMap[start]; !ok {
		return
	}
	note[start] = 0
	//fmt.Printf("%s: %v\n", "initial note", note)
	// map[a:0 b:2147483647 c:2147483647 d:2147483647 e:2147483647]

	for {
		best := ""
		smallest := math.MaxInt
		for z, x := range note {
			if visited[z] {
				continue
			}
			// first loop this if ignored because visited is FALSE
			// second loop skips a
			// third loop skips a and b
			// forth loop skips a, b and c
			// fifth loop skips a, b, c and d
			// sixth loop skips a, b, c, d and e
			if x < smallest {
				smallest = x
				best = z
			}
			// first loop smallest = 0, best = a because 0 is genuinely the smallest
			// second loop smallest = 2, best = b, because when it reaches c, 7 < 2 = false, and b (2) is automatically smallest at the first loop
			// third loop smallest = 5, best = c
			// forth loop smallest = 6, best = d
			// fifth loop smallest = 8, best = e
			// sixth loop smallest = MaxInt, best = ""
		}
		if best == "" || best == destination {
			break
		}
		// exits on sixth loop
		visited[best] = true
		// first loop a visted
		// second loop a and b visited
		// third loop a, b and c visited
		// forth loop a, b, c and d visited
		// fifth loop a, b, c, d and e visited

		for z, x := range fullMap[best] {
			smallestTemp := x + smallest
			if smallestTemp < note[z] {
				note[z] = smallestTemp
				route[z] = best
			}
			// first loop, for b the smallestTemp is 2 ((x = 2) + (smallest = 0)), (smallestTemp = 2) < (note[z] = MaxInt) = true so note[z] move from MaxInt to 2
			// first loop, for c the smallestTemp is 7 ((x = 7) + (smallest = 0)), (smallestTemp = 7) < (note[z] = MaxInt) = true so note[z] move from MaxInt to 7
			// second loop, for c the smallestTemp is 5 ((x = 3) + (smallest = 2)), (smallestTemp = 5) < (note[z] = 7) = true so note[z] move from 7 to 5
			// second loop, for d the smallestTemp is 10 ((x = 8) + (smallest = 2)), (smallestTemp = 10) < (note[z] = MaxInt) = true so note[z] move from MaxInt to 10
			// third loop, for d the smallestTemp is 6 ((x = 1) + (smallest = 5)), (smallestTemp = 6) < (note[z] = 10) = true so note[z] move from 10 to 6
			// second loop, for e the smallestTemp is 11 ((x = 6) + (smallest = 5)), (smallestTemp = 11) < (note[z] = MaxInt) = true so note[z] move from MaxInt to 11
			// forth loop, for e the smallestTemp is 8 ((x = 2) + (smallest = 6)), (smallestTemp = 8) < (note[z] = 11) = true so note[z] move from 11 to 8
			// skips fifth loop {}
		}

		//fmt.Printf("%s: %v\n", "note", note)
		//fmt.Printf("%s: %v\n", "route", route)
		// first loop note = map[a:0 b:2 c:7 d:2147483647 e:2147483647]
		// second loop note = map[a:0 b:2 c:5 d:10 e:2147483647]
		// third loop note = map[a:0 b:2 c:5 d:6 e:11]
		// forth loop note = map[a:0 b:2 c:5 d:6 e:8]
		// fifth loop note = map[a:0 b:2 c:5 d:6 e:8]
		// first route = map[b:a c:a]
		// second route = map[b:a c:b d:b]
		// third route = map[b:a c:b d:c e:c]
		// fourth route = map[b:a c:b d:c e:d]
		// fifth route = map[b:a c:b d:c e:d]
	}
	return
}

func Path(destination string, note map[string]int, route map[string]string) (status, path, pathDistance string, distance int) {
	v, ok := note[destination]
	if v == math.MaxInt || !ok {
		status = "unreachable"
		return
	}
	// exit when the end is MaxInt - default from note (unreachable)

	path = destination
	pathTemp := destination
	//loop from the end
	pathDistance = strconv.Itoa(v)

	for {
		currentRoute, ok := route[pathTemp]
		if !ok {
			break
		}
		path = currentRoute + " -> " + path

		pathDistance = strconv.Itoa(note[currentRoute]) + " -> " + pathDistance

		pathTemp = currentRoute
	}

	status = "reachable"
	distance = v
	return
}
