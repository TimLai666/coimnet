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
//
// Four task variants are provided, selected by Config.Task ("" means
// avoid_obstacles):
//
//   - remember_goal: the goal never appears in the view; instead the first
//     observation's Cue carries the sign of the goal vector and every later
//     Cue is zero, so the agent must pool a single direction reading.
//   - avoid_obstacles: the default behaviour; the goal is visible whenever it
//     falls inside the view cone on a map of random walls.
//   - adapt_after_change: at the step that reaches TimeLimit/2 the walls are
//     redrawn from a fresh random stream; that step's RuleChanged is 1 and
//     every other step's is 0, and the expert follows the new map.
//   - language_goal: the goal is one of the four corners, named each step by
//     a one-hot Token (index order (0,0), (W-1,0), (0,H-1), (W-1,H-1)); the
//     goal stays hidden in the view and the start is a connected free cell
//     different from it.
package nav2d
