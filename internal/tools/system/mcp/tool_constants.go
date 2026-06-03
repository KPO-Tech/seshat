package mcp

// Tool name constants
const ToolNameMCP = "mcp"

// Search hints
const SearchHintMCP = "execute a tool from a connected MCP server"

// Descriptions
const DescriptionMCP = `Executes tools from Model Context Protocol (MCP) servers. MCP tools are provided by external servers and wrapped as native tools. Use ToolSearch to discover available MCP tools.

MCP is an open protocol that allows AI models to interact with external tools and services.
When you call an MCP tool, the request is forwarded to the MCP server which executes the tool and returns the result.

How MCP tools work:
1. MCP servers are configured and connected at startup or dynamically.
2. Each MCP server can provide multiple tools.
3. Tools follow the naming convention mcp__<server>__<tool_name>.
4. Tool schemas are fetched dynamically when needed.

How to use MCP tools:
1. Use ToolSearch to discover available MCP tools from connected servers.
2. MCP tools appear with prefix mcp__ in the tool list.
3. Each tool has its own input schema defined by the MCP server.
4. Execute the tool by passing the required arguments.

Notes:
- MCP tools are read-only unless the tool itself modifies data.
- Each MCP server may have different rate limits.
- Some tools may require authentication configured on the server.`

// MCP tool name prefix
const MCPToolPrefix = "mcp__"
