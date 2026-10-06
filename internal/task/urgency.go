package task

import (
	"sort"
	"time"
)

const (
	urgencyNext       = 15.0
	urgencyDue        = 12.0
	urgencyBlocking   = 8.0
	urgencyScheduled  = 5.0
	urgencyActive     = 4.0
	urgencyAge        = 2.0
	urgencyAnnotation = 1.0
	urgencyTags       = 1.0
	urgencyProject    = 1.0
	urgencyWaiting    = -3.0
	urgencyBlocked    = -5.0
)

// ApplyUrgencyScores calculates derived urgency for every task without
// changing the stored urgency metadata. Dependency inheritance propagates the
// highest downstream score to each blocker, as Taskwarrior urgency.inherit
// does.
func ApplyUrgencyScores(tasks []Task, now time.Time) {
	byID := make(map[string]Task, len(tasks))
	dependents := make(map[string][]string, len(tasks))
	for _, current := range tasks {
		byID[current.ID] = current
		for _, dependencyID := range current.Dependencies {
			dependents[dependencyID] = append(dependents[dependencyID], current.ID)
		}
	}

	base := make(map[string]float64, len(tasks))
	for _, current := range tasks {
		base[current.ID] = baseUrgency(current, now, dependents[current.ID], byID)
	}

	visiting := make(map[string]bool, len(tasks))
	memo := make(map[string]float64, len(tasks))
	var inherited func(string) float64
	inherited = func(id string) float64 {
		if score, ok := memo[id]; ok {
			return score
		}
		if visiting[id] {
			return base[id]
		}
		visiting[id] = true
		score := base[id]
		for _, dependentID := range dependents[id] {
			dependent, ok := byID[dependentID]
			if !ok || dependent.Status == Completed {
				continue
			}
			if downstream := inherited(dependentID); downstream > score {
				score = downstream
			}
		}
		visiting[id] = false
		memo[id] = score
		return score
	}

	for index := range tasks {
		tasks[index].UrgencyScore = inherited(tasks[index].ID)
	}
}

// SortByUrgency orders tasks by derived urgency with deterministic position,
// creation-time, and UUID tie-breakers.
func SortByUrgency(tasks []Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].UrgencyScore != tasks[j].UrgencyScore {
			return tasks[i].UrgencyScore > tasks[j].UrgencyScore
		}
		if tasks[i].Position != tasks[j].Position {
			return tasks[i].Position < tasks[j].Position
		}
		if tasks[i].CreatedAt != tasks[j].CreatedAt {
			return tasks[i].CreatedAt < tasks[j].CreatedAt
		}
		return tasks[i].ID < tasks[j].ID
	})
}

func baseUrgency(current Task, now time.Time, dependentIDs []string, byID map[string]Task) float64 {
	score := priorityUrgency(current.Priority)
	if current.Status == Active {
		score += urgencyActive
	}
	if current.DueAt != "" {
		score += dueFactor(current.DueAt, now) * urgencyDue
	}
	if current.CreatedAt != "" {
		score += ageFactor(current.CreatedAt, now) * urgencyAge
	} else {
		score += urgencyAge
	}
	if current.AnnotationCount > 0 {
		score += annotationFactor(current.AnnotationCount) * urgencyAnnotation
	}
	if current.InitiativeName != "" || current.ProjectID != "" {
		score += urgencyProject
	}
	if current.WaitUntil != "" && current.WaitUntil > now.UTC().Format("2006-01-02") {
		score += urgencyWaiting
	}
	if isBlocked(current, byID) {
		score += urgencyBlocked
	}
	for _, dependentID := range dependentIDs {
		dependent, ok := byID[dependentID]
		if ok && dependent.Status != Completed {
			score += urgencyBlocking
			break
		}
	}
	return score
}

func priorityUrgency(priority string) float64 {
	switch priority {
	case "H":
		return 6.0
	case "M":
		return 3.9
	case "L":
		return 1.8
	default:
		return 0
	}
}

func dueFactor(value string, now time.Time) float64 {
	due, err := time.ParseInLocation("2006-01-02", value, time.UTC)
	if err != nil {
		return 0
	}
	daysOverdue := now.UTC().Sub(due).Hours() / 24
	if daysOverdue >= 7 {
		return 1
	}
	if daysOverdue >= -14 {
		return ((daysOverdue + 14) * 0.8 / 21) + 0.2
	}
	return 0.2
}

func ageFactor(value string, now time.Time) float64 {
	created, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 1
	}
	age := int(now.UTC().Sub(created).Hours() / 24)
	if age <= 0 {
		return 0
	}
	if age >= 365 {
		return 1
	}
	return float64(age) / 365
}

func annotationFactor(count int) float64 {
	switch {
	case count == 1:
		return 0.8
	case count == 2:
		return 0.9
	default:
		return 1
	}
}

func isBlocked(current Task, byID map[string]Task) bool {
	for _, dependencyID := range current.Dependencies {
		dependency, ok := byID[dependencyID]
		if !ok || dependency.Status != Completed {
			return true
		}
	}
	return false
}
