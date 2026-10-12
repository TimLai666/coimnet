package learning

import (
	"fmt"

	"github.com/TimLai666/coimnet/modulation"
)

// ChemicalSourceState is aligned with ChemistryConfig.Sources. Only controller
// sources carry an activity window; other entries are empty. An absent list is
// the legacy fresh-source boundary. When present its length and every source
// kind must match, and every controller entry must carry a complete valid state.
type ChemicalSourceState struct {
	Controller *modulation.ControllerState `json:"controller,omitempty"`
}

// sourceStates returns an owned boundary. Fresh controller windows are emitted
// as full zero rows so strict checkpoints never contain required null arrays.
// It returns nil when no source has mutable state, preserving the old wire.
func (r *chemicalRuntime) sourceStates() []ChemicalSourceState {
	if r == nil || r.sources == nil {
		return nil
	}
	states := make([]ChemicalSourceState, len(r.sources))
	mutable := false
	for index, source := range r.sources {
		controller, ok := source.(*modulation.Controller)
		if !ok || controller == nil {
			continue
		}
		mutable = true
		state := controller.State()
		if state.Window == nil {
			state = freshControllerState(controller)
		}
		states[index].Controller = &state
	}
	if !mutable {
		return nil
	}
	return states
}

// restoreSourceStates validates all source boundaries before replacing any
// source. It accepts an absent legacy list as a fresh boundary and owns all
// restored arrays. Malformed kinds, lengths or windows leave sources unchanged.
func (r *chemicalRuntime) restoreSourceStates(states []ChemicalSourceState) error {
	if r == nil {
		return fmt.Errorf("cannot restore source states into a nil chemical runtime")
	}
	if len(r.config.Sources) != len(r.sources) {
		return fmt.Errorf("source declaration count %d does not match source runtime count %d", len(r.config.Sources), len(r.sources))
	}
	if states != nil && len(states) != len(r.sources) {
		return fmt.Errorf("source state count %d does not match source count %d", len(states), len(r.sources))
	}

	// Every controller is cloned before any state is validated or restored. A
	// failed later entry therefore cannot mutate the receiver's window or its
	// last-release cache through an earlier candidate.
	candidate := r.cloneSources()
	for index, source := range candidate {
		wantController := r.config.Sources[index].Kind == modulation.SourceController
		controller, gotController := source.(*modulation.Controller)
		if wantController != gotController || (wantController && controller == nil) {
			return fmt.Errorf("source %d kind %q does not match its built source", index, r.config.Sources[index].Kind)
		}
	}

	if states == nil {
		// An absent list is the pre-SourceStates wire: only source memory is
		// fresh. The caller has already copied the concentration and neural
		// state, so neither clock is changed here.
		for index, source := range candidate {
			controller, ok := source.(*modulation.Controller)
			if !ok {
				continue
			}
			if err := controller.RestoreState(freshControllerState(controller)); err != nil {
				return fmt.Errorf("source %d controller fresh state: %w", index, err)
			}
		}
		r.sources = candidate
		return nil
	}

	for index, state := range states {
		controller, isController := candidate[index].(*modulation.Controller)
		if isController {
			if state.Controller == nil {
				return fmt.Errorf("source %d controller state is missing", index)
			}
			// A nil window is accepted only by Controller.RestoreState's legacy
			// API. Once SourceStates is present, it is a damaged new boundary.
			if state.Controller.Window == nil {
				return fmt.Errorf("source %d controller state has a nil window", index)
			}
			if err := controller.RestoreState(*state.Controller); err != nil {
				return fmt.Errorf("source %d controller state: %w", index, err)
			}
			continue
		}
		if state.Controller != nil {
			return fmt.Errorf("source %d kind %q carries controller state", index, r.config.Sources[index].Kind)
		}
	}
	r.sources = candidate
	return nil
}

// cloneSources gives an operation private mutable source state. Immutable
// source declarations may be reused; controller declarations, window and last
// release cache are deeply cloned. No candidate shares mutable controller data.
func (r *chemicalRuntime) cloneSources() []modulation.Source {
	if r == nil || r.sources == nil {
		return nil
	}
	owned := make([]modulation.Source, len(r.sources))
	for index, source := range r.sources {
		if controller, ok := source.(*modulation.Controller); ok {
			owned[index] = controller.Clone()
		} else {
			owned[index] = source
		}
	}
	return owned
}

func freshControllerState(controller *modulation.Controller) modulation.ControllerState {
	state := modulation.ControllerState{}
	if controller == nil {
		return state
	}
	state.Window = make([][]float64, controller.Inputs.SummaryWindow)
	for row := range state.Window {
		state.Window[row] = make([]float64, controller.Nodes)
	}
	return state
}
