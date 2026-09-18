// Package nav2d provides the two-dimensional grid navigation environment of
// ticket 28 stage 1: a walled map, a front-facing view cone, heading and
// movement actions, collisions, goal and time-limit termination, and a BFS
// expert that follows a shortest path. See the type and method documentation
// in env.go for the public contract.
//
// Coordinates: x grows to the right, y grows downward, and one map cell is one
// unit. A heading is an angle in degrees: 0 faces +x (east), 90 faces -y
// (north), 180 faces -x (west), 270 faces +y (south). The "left" side of the
// view cone is the heading rotated counterclockwise by 90 degrees: facing
// east the left is north, facing north the left is west, and so on.
//
// Relative versus absolute: the Observation stays entirely relative to the
// agent - the view cone ahead, the heading one-hot, and the last step's
// collision and step counters. The position, the goal coordinates, the path
// length and the collision count appear only in the evaluator-only Info and
// never enter an encoder.
package nav2d
