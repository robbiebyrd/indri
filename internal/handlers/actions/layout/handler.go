package layout

import (
	"fmt"

	"github.com/robbiebyrd/indri/internal/injector"
	"github.com/robbiebyrd/indri/internal/models"
	"github.com/robbiebyrd/indri/internal/services/mutation"
	"github.com/robbiebyrd/indri/internal/transport"
	handlerUtils "github.com/robbiebyrd/indri/internal/handlers/utils"
)

type Handler struct {
	i *injector.Injector
}

func New(i *injector.Injector) *Handler {
	return &Handler{i}
}

// Handle processes a layout mutation request. The caller must be the host of the
// named game; auth is always resolved from the caller's own connection key, never
// from a client-supplied userId.
func (h *Handler) Handle(s transport.Conn, decodedMsg map[string]interface{}) error {
	gameCode, err := handlerUtils.RequireGameCode(decodedMsg)
	if err != nil {
		return err
	}

	g, _, err := handlerUtils.RequireHost(h.i, s, *gameCode)
	if err != nil {
		return err
	}
	gameId := g.ID

	op, err := DecodeOp(decodedMsg)
	if err != nil {
		return err
	}

	// Mutate diffs before/after and publishes automatically — do not publish here.
	return h.i.GameRepo.Mutate(gameId, func(g *models.Game) error {
		return applyLayoutOp(g, op)
	})
}

// applyLayoutOp applies a single layout op to the game's PublicData["layout"]
// map. It returns mutation.ErrAbort for no-op cases so Mutate skips the write.
// It never touches Players, Teams, Stage, or PrivateData.
func applyLayoutOp(g *models.Game, op *Op) error {
	if g.PublicData == nil {
		g.PublicData = map[string]interface{}{}
	}

	layout, _ := g.PublicData["layout"].(map[string]interface{})
	if layout == nil {
		layout = map[string]interface{}{}
	}

	if err := applyOp(layout, op); err != nil {
		return err
	}

	if err := ValidateLayout(layout); err != nil {
		return err
	}

	g.PublicData["layout"] = layout
	return nil
}

// applyOp mutates the layout map according to op.
func applyOp(layout map[string]interface{}, op *Op) error {
	switch op.Op {
	case "addWidget":
		return applyAddWidget(layout, op)
	case "removeWidget":
		return applyRemoveWidget(layout, op)
	case "setPlacement":
		return applySetPlacement(layout, op)
	case "setWidgetConfig":
		return applySetWidgetConfig(layout, op)
	case "setStyle":
		return applySetStyle(layout, op)
	case "setGrid":
		return applySetGrid(layout, op)
	case "setScript":
		return applySetScript(layout, op)
	default:
		// DecodeOp already rejects unknown ops, so this is a programming error.
		return fmt.Errorf("applyLayoutOp: unhandled op %q", op.Op)
	}
}

// ensureScenes returns the scenes map from layout, creating it if absent.
func ensureScenes(layout map[string]interface{}) map[string]interface{} {
	scenes, _ := layout["scenes"].(map[string]interface{})
	if scenes == nil {
		scenes = map[string]interface{}{}
		layout["scenes"] = scenes
	}
	return scenes
}

// ensureScene returns the scene map for sceneID, creating it if absent.
func ensureScene(layout map[string]interface{}, sceneID string) map[string]interface{} {
	scenes := ensureScenes(layout)
	scene, _ := scenes[sceneID].(map[string]interface{})
	if scene == nil {
		scene = map[string]interface{}{}
		scenes[sceneID] = scene
	}
	return scene
}

// ensureWidgets returns the widgets map for sceneID's scene, creating it if absent.
func ensureWidgets(layout map[string]interface{}, sceneID string) map[string]interface{} {
	scene := ensureScene(layout, sceneID)
	widgets, _ := scene["widgets"].(map[string]interface{})
	if widgets == nil {
		widgets = map[string]interface{}{}
		scene["widgets"] = widgets
	}
	return widgets
}

// getWidgets returns the widgets map for sceneID, or nil if the scene or its
// widgets map doesn't exist. Used for read-only access (e.g. removeWidget).
func getWidgets(layout map[string]interface{}, sceneID string) map[string]interface{} {
	scenes, _ := layout["scenes"].(map[string]interface{})
	if scenes == nil {
		return nil
	}
	scene, _ := scenes[sceneID].(map[string]interface{})
	if scene == nil {
		return nil
	}
	widgets, _ := scene["widgets"].(map[string]interface{})
	return widgets
}

func applyAddWidget(layout map[string]interface{}, op *Op) error {
	widgets := ensureWidgets(layout, op.SceneID)
	widgets[op.WidgetID] = op.Widget
	return nil
}

func applyRemoveWidget(layout map[string]interface{}, op *Op) error {
	widgets := getWidgets(layout, op.SceneID)
	if widgets == nil {
		return mutation.ErrAbort
	}
	if _, exists := widgets[op.WidgetID]; !exists {
		return mutation.ErrAbort
	}
	delete(widgets, op.WidgetID)
	return nil
}

func applySetPlacement(layout map[string]interface{}, op *Op) error {
	widgets := ensureWidgets(layout, op.SceneID)
	widget, _ := widgets[op.WidgetID].(map[string]interface{})
	if widget == nil {
		widget = map[string]interface{}{}
	}
	widget["placement"] = op.Placement
	widgets[op.WidgetID] = widget
	return nil
}

func applySetWidgetConfig(layout map[string]interface{}, op *Op) error {
	widgets := ensureWidgets(layout, op.SceneID)
	widget, _ := widgets[op.WidgetID].(map[string]interface{})
	if widget == nil {
		widget = map[string]interface{}{}
	}
	existing, _ := widget["config"].(map[string]interface{})
	if existing == nil {
		existing = map[string]interface{}{}
	}
	for k, v := range op.Config {
		existing[k] = v
	}
	widget["config"] = existing
	widgets[op.WidgetID] = widget
	return nil
}

func applySetStyle(layout map[string]interface{}, op *Op) error {
	switch op.Scope {
	case "board":
		layout["style"] = op.Style
	case "scene":
		scene := ensureScene(layout, op.SceneID)
		scene["style"] = op.Style
	case "widget":
		widgets := ensureWidgets(layout, op.SceneID)
		widget, _ := widgets[op.WidgetID].(map[string]interface{})
		if widget == nil {
			widget = map[string]interface{}{}
		}
		widget["style"] = op.Style
		widgets[op.WidgetID] = widget
	default:
		return fmt.Errorf("setStyle: unknown scope %q", op.Scope)
	}
	return nil
}

func applySetGrid(layout map[string]interface{}, op *Op) error {
	layout["grid"] = op.Grid
	return nil
}

func applySetScript(layout map[string]interface{}, op *Op) error {
	switch op.Scope {
	case "board":
		layout["script"] = *op.Source
	case "scene":
		scene := ensureScene(layout, op.SceneID)
		scene["script"] = *op.Source
	case "widget":
		widgets := ensureWidgets(layout, op.SceneID)
		widget, _ := widgets[op.WidgetID].(map[string]interface{})
		if widget == nil {
			widget = map[string]interface{}{}
		}
		widget["script"] = *op.Source
		widgets[op.WidgetID] = widget
	default:
		return fmt.Errorf("setScript: unknown scope %q", op.Scope)
	}
	return nil
}
