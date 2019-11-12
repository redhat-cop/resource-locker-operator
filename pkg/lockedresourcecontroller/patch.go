package lockedresourcecontroller

import (
	"encoding/json"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func filterOutPaths(obj *unstructured.Unstructured, jsonPaths []string) (*unstructured.Unstructured, error) {
	patches, err := createPatchesFromJsonPaths(jsonPaths)
	if err != nil {
		return &unstructured.Unstructured{}, err
	}
	patch, err := jsonpatch.DecodePatch(patches)
	if err != nil {
		return &unstructured.Unstructured{}, err
	}

	original, err := obj.MarshalJSON()
	if err != nil {
		return &unstructured.Unstructured{}, err
	}

	modified, err := patch.Apply(original)
	if err != nil {
		return &unstructured.Unstructured{}, err
	}

	var result = &unstructured.Unstructured{}

	err = result.UnmarshalJSON(modified)

	if err != nil {
		return &unstructured.Unstructured{}, err
	}

	return result, nil
}

type patch struct {
	operation string `json:"op"`
	path      string `json:"path"`
}

func createPatchesFromJsonPaths(jsonPaths []string) ([]byte, error) {
	patches := []patch{}
	for _, jsonPath := range jsonPaths {
		patches = append(patches, patch{
			operation: "remove",
			path:      getMergePathFromJsonPath(jsonPath),
		})
	}
	return json.Marshal(patches)
}

func getMergePathFromJsonPath(jsonPath string) string {
	//remove "$" if present
	if strings.HasPrefix(jsonPath, "$") {
		jsonPath = jsonPath[1:]
	}
	// convert "[" and "]" to "."
	if strings.HasSuffix(jsonPath, "]") {
		jsonPath = jsonPath[:len(jsonPath)-2]
	}
	jsonPath = strings.ReplaceAll(jsonPath, "[", ".")
	jsonPath = strings.ReplaceAll(jsonPath, "]", ".")
	// convert "." to "/"
	jsonPath = strings.ReplaceAll(jsonPath, ".", "/")
	return jsonPath
}
