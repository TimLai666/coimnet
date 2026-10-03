package main

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

type trialSplit struct {
	Train      []string `json:"train_trial_ids"`
	Validation []string `json:"validation_trial_ids"`
	Test       []string `json:"test_trial_ids"`
}

const splitSeed = "20261003"

// makeSplit validates the existing 26/13 partition, then selects validation
// trials only from the original training group. The smaller-fixture rule uses
// the same six-of-26 ratio rounded to the nearest integer, with at least one
// validation trial and at least one remaining training trial.
func makeSplit(trials []historyTrial, originalTrain, originalTest []string) (trialSplit, error) {
	known, err := indexTrials(trials)
	if err != nil {
		return trialSplit{}, err
	}
	if len(originalTrain) == 0 || len(originalTest) == 0 {
		return trialSplit{}, fmt.Errorf("realnavmemory: original train and test groups must be non-empty")
	}
	if err := validateIDGroup("original train", originalTrain, known); err != nil {
		return trialSplit{}, err
	}
	if err := validateIDGroup("original test", originalTest, known); err != nil {
		return trialSplit{}, err
	}
	seen := make(map[string]string, len(originalTrain)+len(originalTest))
	for _, id := range originalTrain {
		seen[id] = "train"
	}
	for _, id := range originalTest {
		if previous, exists := seen[id]; exists {
			return trialSplit{}, fmt.Errorf("realnavmemory: original train/test groups overlap on trial %q (already in %s)", id, previous)
		}
		seen[id] = "test"
	}
	if len(seen) != len(known) {
		return trialSplit{}, fmt.Errorf("realnavmemory: original train/test groups do not fully cover all trials: got %d, want %d", len(seen), len(known))
	}
	if len(originalTrain) < 3 {
		return trialSplit{}, fmt.Errorf("realnavmemory: original training fixture must contain at least 3 trials")
	}

	validationCount := int(float64(len(originalTrain)*6)/26.0 + 0.5)
	if validationCount < 1 {
		validationCount = 1
	}
	if validationCount >= len(originalTrain) {
		validationCount = len(originalTrain) - 1
	}

	byCondition := make(map[string][]string)
	for _, id := range originalTrain {
		condition := known[id].Condition
		byCondition[condition] = append(byCondition[condition], id)
	}
	conditions := make([]string, 0, len(byCondition))
	for condition := range byCondition {
		conditions = append(conditions, condition)
	}
	sort.Strings(conditions)

	type allocation struct {
		condition string
		count     int
		remainder int
	}
	allocations := make([]allocation, 0, len(conditions))
	baseTotal := 0
	for _, condition := range conditions {
		groupSize := len(byCondition[condition])
		product := validationCount * groupSize
		base := product / len(originalTrain)
		allocations = append(allocations, allocation{condition: condition, count: base, remainder: product % len(originalTrain)})
		baseTotal += base
	}
	sort.Slice(allocations, func(i, j int) bool {
		if allocations[i].remainder != allocations[j].remainder {
			return allocations[i].remainder > allocations[j].remainder
		}
		return allocations[i].condition < allocations[j].condition
	})
	for i := 0; i < validationCount-baseTotal; i++ {
		allocations[i].count++
	}
	allocationByCondition := make(map[string]int, len(allocations))
	for _, allocation := range allocations {
		allocationByCondition[allocation.condition] = allocation.count
	}

	validation := make(map[string]struct{}, validationCount)
	for _, condition := range conditions {
		ids := append([]string(nil), byCondition[condition]...)
		sort.Slice(ids, func(i, j int) bool {
			iHash := sha256.Sum256([]byte(splitSeed + "\x1f" + ids[i]))
			jHash := sha256.Sum256([]byte(splitSeed + "\x1f" + ids[j]))
			if string(iHash[:]) != string(jHash[:]) {
				return string(iHash[:]) < string(jHash[:])
			}
			return ids[i] < ids[j]
		})
		for _, id := range ids[:allocationByCondition[condition]] {
			validation[id] = struct{}{}
		}
	}

	train := make([]string, 0, len(originalTrain)-len(validation))
	validationIDs := make([]string, 0, len(validation))
	for _, id := range originalTrain {
		if _, selected := validation[id]; selected {
			validationIDs = append(validationIDs, id)
		} else {
			train = append(train, id)
		}
	}
	sort.Strings(train)
	sort.Strings(validationIDs)
	test := append([]string(nil), originalTest...)
	split := trialSplit{Train: train, Validation: validationIDs, Test: test}
	if err := validateSplit(trials, split); err != nil {
		return trialSplit{}, fmt.Errorf("realnavmemory: generated split: %w", err)
	}
	return split, nil
}

// validateSplit requires the three output groups to be non-empty, mutually
// exclusive and an exact cover of the supplied history trials.
func validateSplit(trials []historyTrial, split trialSplit) error {
	known, err := indexTrials(trials)
	if err != nil {
		return err
	}
	if len(split.Train) == 0 || len(split.Validation) == 0 || len(split.Test) == 0 {
		return fmt.Errorf("realnavmemory: split train, validation and test groups must be non-empty")
	}
	assigned := make(map[string]string, len(known))
	groups := []struct {
		name string
		ids  []string
	}{
		{name: "train", ids: split.Train},
		{name: "validation", ids: split.Validation},
		{name: "test", ids: split.Test},
	}
	for _, group := range groups {
		seenInGroup := make(map[string]struct{}, len(group.ids))
		for _, id := range group.ids {
			if id == "" {
				return fmt.Errorf("realnavmemory: %s group contains an empty trial id", group.name)
			}
			if _, exists := known[id]; !exists {
				return fmt.Errorf("realnavmemory: %s group contains unknown trial %q", group.name, id)
			}
			if _, duplicate := seenInGroup[id]; duplicate {
				return fmt.Errorf("realnavmemory: %s group contains duplicate trial %q", group.name, id)
			}
			seenInGroup[id] = struct{}{}
			if previous, exists := assigned[id]; exists {
				return fmt.Errorf("realnavmemory: split groups overlap on trial %q between %s and %s", id, previous, group.name)
			}
			assigned[id] = group.name
		}
	}
	if len(assigned) != len(known) {
		return fmt.Errorf("realnavmemory: split does not fully cover trials: got %d, want %d", len(assigned), len(known))
	}
	return nil
}

// selectTrials returns deep copies in the caller's ID order so the original
// history and the selected split cannot share mutable step storage.
func selectTrials(trials []historyTrial, ids []string) ([]historyTrial, error) {
	known, err := indexTrials(trials)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("realnavmemory: selected trial IDs must be non-empty")
	}
	seen := make(map[string]struct{}, len(ids))
	selected := make([]historyTrial, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("realnavmemory: selected trial IDs contain an empty trial id")
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("realnavmemory: selected trial IDs contain duplicate %q", id)
		}
		trial, exists := known[id]
		if !exists {
			return nil, fmt.Errorf("realnavmemory: selected trial IDs contain unknown trial %q", id)
		}
		seen[id] = struct{}{}
		copyTrial := trial
		copyTrial.Steps = append([]historyStep(nil), trial.Steps...)
		selected = append(selected, copyTrial)
	}
	return selected, nil
}

func indexTrials(trials []historyTrial) (map[string]historyTrial, error) {
	if len(trials) == 0 {
		return nil, fmt.Errorf("realnavmemory: no history trials")
	}
	indexed := make(map[string]historyTrial, len(trials))
	for _, trial := range trials {
		if trial.ID == "" {
			return nil, fmt.Errorf("realnavmemory: history trial ID is empty")
		}
		if trial.Condition == "" {
			return nil, fmt.Errorf("realnavmemory: history trial %q has empty condition", trial.ID)
		}
		if _, duplicate := indexed[trial.ID]; duplicate {
			return nil, fmt.Errorf("realnavmemory: duplicate history trial %q", trial.ID)
		}
		indexed[trial.ID] = trial
	}
	return indexed, nil
}

func validateIDGroup(name string, ids []string, known map[string]historyTrial) error {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return fmt.Errorf("realnavmemory: %s group contains an empty trial id", name)
		}
		if _, exists := known[id]; !exists {
			return fmt.Errorf("realnavmemory: %s group contains unknown trial %q", name, id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("realnavmemory: %s group contains duplicate trial %q", name, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}
