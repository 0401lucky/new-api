package zhipu_4v

import "testing"

func TestModelListIncludesLatestZhipuModels(t *testing.T) {
	models := map[string]bool{}
	for _, modelName := range ModelList {
		models[modelName] = true
	}

	for _, modelName := range []string{"glm-5.2", "glm-5.1", "glm-5-turbo", "glm-5v-turbo", "glm-image", "cogview-4"} {
		if !models[modelName] {
			t.Fatalf("ModelList missing %q", modelName)
		}
	}
}
