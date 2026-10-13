package gateway

// magpie's image recognition, the model that describes images to a model
// that can't see them (seer: Settings › Models › Image recognition, else
// AutoVision's), is also an MCP server of its own beside the web search's
// (JonahFu on X), so an agent on a model that can't see, or any agent at
// all, can mount it as an analyze_image tool: Streamable HTTP at
// VisionMCPPath, served and guarded as SearchMCPPath is (serveMCP), a
// gateway key's limit holding each look (budget.Reserve).

// VisionMCPPath is where the image recognition MCP server is served.
const VisionMCPPath = "/mcp/magpie/vision"

const visionMCPTool = "analyze_image"

// visionMCPBody is how long a message to the server may be: an image is
// sent in it as a data URL.
const visionMCPBody = 32 << 20

// visionFileMax is the largest image file the tool reads from a path.
const visionFileMax = 20 << 20

// visionAskSystem is what the model is told when the agent asks something
// of the image, in place of visionSystem's full description.
const visionAskSystem = `You look at an image for an AI agent that cannot see it, and answer what it asks about the image from what the image shows.
- Transcribe text exactly as written wherever the answer needs it: code, terminal output, error messages, UI labels, numbers.
- Say so when what is asked can't be read or isn't in the image; don't guess.
No preamble.`

var visionMCPSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"image": map[string]any{
			"type":        "string",
			"description": "The image: an absolute path to an image file on this computer (PNG, JPEG, GIF or WebP), an http(s) URL, or a data URL (data:image/png;base64,...).",
		},
		"prompt": map[string]any{
			"type":        "string",
			"description": "What to find out about the image. Without it the image is described in full: its text transcribed, its layout, charts and diagrams.",
		},
	},
	"required": []string{"image"},
}
