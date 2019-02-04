package gantry // import "github.com/ad-freiburg/gantry"
// Adapted version of https://github.com/looplab/tarjan/blob/master/tarjan.go
import (
	"fmt"
	"strings"
)

type tarjanData struct {
	tarjan
	nodes []tarjanNode
	stack []Step
	index map[string]int
}

type tarjan struct {
	graph  map[string]Step
	output [][]Step
}

type tarjanNode struct {
	lowlink int
	stacked bool
}

func (td *tarjanData) strongConnect(v string) (*tarjanNode, error) {
	index := len(td.nodes)
	td.index[v] = index
	td.stack = append(td.stack, td.graph[v])
	td.nodes = append(td.nodes, tarjanNode{lowlink: index, stacked: true})
	node := &td.nodes[index]

	deps, err := td.graph[v].Dependencies()
	if err != nil {
		return nil, err
	}
	for w, _ := range *deps {
		if _, ok := td.graph[w]; !ok {
			return nil, fmt.Errorf("Unknown dependency '%s' for step '%s'", w, v)
		}
		i, seen := td.index[w]
		if !seen {
			n, err := td.strongConnect(w)
			if err != nil {
				return nil, err
			}
			if n.lowlink < node.lowlink {
				node.lowlink = n.lowlink
			}
		} else if td.nodes[i].stacked {
			if i < node.lowlink {
				node.lowlink = i
			}
		}
	}

	if node.lowlink == index {
		var vertices []Step
		i := len(td.stack) - 1
		for {
			w := td.stack[i]
			stackIndex := td.index[w.Name()]
			td.nodes[stackIndex].stacked = false
			vertices = append(vertices, w)
			if stackIndex == index {
				break
			}
			i--
		}
		td.stack = td.stack[:i]
		td.output = append(td.output, vertices)
	}
	return node, nil
}

func NewTarjan(steps map[string]Step) (*tarjan, error) {
	// Determine components and topological order
	t := &tarjanData{
		nodes: make([]tarjanNode, 0, len(steps)),
		index: make(map[string]int, len(steps)),
	}
	t.graph = steps
	for v := range t.graph {
		if _, ok := t.index[v]; !ok {
			_, err := t.strongConnect(v)
			if err != nil {
				return nil, err
			}
		}
	}
	return &t.tarjan, nil
}

func (t *tarjan) Parse() (*pipelines, error) {
	result := make(pipelines, 0)
	// walk reverse order, if all requirements are found the next step is a new component
	resultIndex := 0
	requirements := make(map[string]bool, 0)
	for i := len(t.output) - 1; i >= 0; i-- {
		steps := t.output[i]
		if len(steps) > 1 {
			names := make([]string, len(steps))
			for i, step := range steps {
				names[i] = step.Name()
			}
			return nil, fmt.Errorf("cyclic component found in (sub)pipeline: '%s'", strings.Join(names, ", "))
		}
		var step = steps[0]
		dependencies, _ := step.Dependencies()
		for r, _ := range *dependencies {
			requirements[r] = true
		}
		delete(requirements, step.Name())
		if len(result)-1 < resultIndex {
			result = append(result, make([]Step, 0))
		}
		result[resultIndex] = append([]Step{step}, result[resultIndex]...)
		if len(requirements) == 0 {
			resultIndex++
		}
	}
	return &result, nil
}