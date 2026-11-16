package tagger

import (
	"bufio"
	"math"
	"os"
	"sort"
	"strings"
)

// Scene tags.
const (
	TagBeach      = "plage"
	TagMountain   = "montagne"
	TagCity       = "ville"
	TagCountry    = "campagne"
	TagCastle     = "château"
	TagChurch     = "église"
	sceneMinScore = 0.30
)

// sceneGroups maps Places365 categories onto the broad place tags.
var sceneGroups = map[string][]string{
	TagBeach:    {"beach", "coast", "ocean", "lagoon", "wave", "islet"},
	TagMountain: {"mountain", "mountain_path", "mountain_snowy", "volcano", "glacier", "valley", "cliff", "snowfield", "ski_slope", "butte", "canyon"},
	TagCity:     {"street", "downtown", "skyscraper", "plaza", "alley", "crosswalk", "building_facade", "residential_neighborhood", "promenade", "viaduct", "apartment_building/outdoor"},
	TagCountry:  {"field/cultivated", "field/wild", "farm", "pasture", "vineyard", "orchard", "barn", "hayfield", "wheat_field", "corn_field", "field_road", "rice_paddy", "vegetable_garden", "windmill", "wind_farm", "village", "tree_farm"},
	TagCastle:   {"castle", "palace", "moat/water"},
	TagChurch:   {"church/indoor", "church/outdoor"},
}

// outdoorLabels are categories that imply a visible sky.
var outdoorLabels = map[string]bool{"sky": true, "mountain": true, "mountain_snowy": true, "beach": true, "coast": true, "ocean": true,
	"field/cultivated": true, "field/wild": true, "pasture": true, "vineyard": true, "farm": true, "castle": true, "palace": true,
	"church/outdoor": true, "street": true, "downtown": true, "skyscraper": true, "plaza": true, "village": true, "valley": true,
	"glacier": true, "volcano": true, "cliff": true, "lagoon": true, "harbor": true, "pier": true, "hayfield": true, "wheat_field": true,
	"orchard": true, "park": true, "lawn": true, "tower": true, "bridge": true, "snowfield": true, "ski_slope": true, "canyon": true}

func loadCategories(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var labels []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if len(name) > 3 && name[0] == '/' && name[2] == '/' {
			name = name[3:]
		}
		labels = append(labels, name)
	}
	return labels, scanner.Err()
}

func softmax(logits []float32) []float64 {
	maxValue := float32(math.Inf(-1))
	for _, v := range logits {
		if v > maxValue {
			maxValue = v
		}
	}
	out := make([]float64, len(logits))
	sum := 0.0
	for i, v := range logits {
		out[i] = math.Exp(float64(v - maxValue))
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// sceneTags turns category probabilities into place tags. outdoor reports
// whether the scene is probably outdoors.
func sceneTags(labels []string, probs []float64) (tags []string, outdoor bool) {
	byLabel := make(map[string]float64, len(labels))
	outdoorScore := 0.0
	for i, label := range labels {
		if i < len(probs) {
			byLabel[label] = probs[i]
			if outdoorLabels[label] {
				outdoorScore += probs[i]
			}
		}
	}
	type scored struct {
		tag   string
		score float64
	}
	var found []scored
	for tag, members := range sceneGroups {
		total := 0.0
		for _, member := range members {
			total += byLabel[member]
		}
		if total >= sceneMinScore {
			found = append(found, scored{tag, total})
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].score > found[j].score })
	for _, f := range found {
		tags = append(tags, f.tag)
	}
	return tags, outdoorScore >= 0.5
}
