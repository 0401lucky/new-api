package common

import "testing"

func TestIsImageGenerationModelIncludesZhipuImages(t *testing.T) {
	for _, modelName := range []string{"glm-image", "cogview-4"} {
		if !IsImageGenerationModel(modelName) {
			t.Fatalf("IsImageGenerationModel(%q) = false, want true", modelName)
		}
	}
}
