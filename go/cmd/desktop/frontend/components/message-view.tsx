import { For, Show, createMemo, JSX } from "solid-js"

interface ToolCall {
  name: string
  input: string
  output?: string
  status: "pending" | "running" | "done" | "error"
}

interface MessagePart {
  type: "text" | "code" | "tool"
  content?: string
  language?: string
  tool?: ToolCall
}

interface Message {
  id: string
  role: "user" | "assistant"
  parts: MessagePart[]
  timestamp: number
}

interface MessageViewProps {
  message: Message
}

export function MessageView(props: MessageViewProps) {
  return (
    <div
      classList={{
        "flex gap-3 group": true,
        "flex-row-reverse": props.message.role === "user"
      }}
    >
      {/* Avatar */}
      <div
        classList={{
          "w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0": true,
          "bg-accent-primary": props.message.role === "user",
          "bg-gradient-to-br from-purple-500 to-blue-500": props.message.role === "assistant"
        }}
      >
        <Show
          when={props.message.role === "assistant"}
          fallback={
            <svg class="w-4 h-4 text-white" fill="currentColor" viewBox="0 0 24 24">
              <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 3c1.66 0 3 1.34 3 3s-1.34 3-3 3-3-1.34-3-3 1.34-3 3-3zm0 14.2c-2.5 0-4.71-1.28-6-3.22.03-1.99 4-3.08 6-3.08 1.99 0 5.97 1.09 6 3.08-1.29 1.94-3.5 3.22-6 3.22z"/>
            </svg>
          }
        >
          <svg class="w-4 h-4 text-white" fill="currentColor" viewBox="0 0 24 24">
            <path d="M13 3L4 14h7l-2 7 9-11h-7l2-7z" />
          </svg>
        </Show>
      </div>

      {/* Content */}
      <div class="flex-1 min-w-0 space-y-2">
        <For each={props.message.parts}>
          {(part) => (
            <Show when={part.type === "text"}>
              <TextPart content={part.content || ""} />
            </Show>
          )}
        </For>
        <For each={props.message.parts}>
          {(part) => (
            <Show when={part.type === "code"}>
              <CodePart content={part.content || ""} language={part.language} />
            </Show>
          )}
        </For>
        <For each={props.message.parts}>
          {(part) => (
            <Show when={part.type === "tool" && part.tool}>
              <ToolPart tool={part.tool!} />
            </Show>
          )}
        </For>

        {/* Timestamp */}
        <div class="text-xs text-text-weaker opacity-0 group-hover:opacity-100 transition-opacity">
          {new Date(props.message.timestamp).toLocaleTimeString()}
        </div>
      </div>
    </div>
  )
}

function TextPart(props: { content: string }) {
  // Simple markdown-like parsing
  const rendered = createMemo(() => {
    const lines = props.content.split('\n')
    const elements: JSX.Element[] = []

    let inCodeBlock = false
    let codeContent: string[] = []
    let codeLang = ""

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i]

      // Code block start/end
      if (line.startsWith('```')) {
        if (!inCodeBlock) {
          inCodeBlock = true
          codeLang = line.slice(3).trim()
          codeContent = []
        } else {
          inCodeBlock = false
          elements.push(
            <CodePart content={codeContent.join('\n')} language={codeLang} />
          )
        }
        continue
      }

      if (inCodeBlock) {
        codeContent.push(line)
        continue
      }

      // Headers
      if (line.startsWith('### ')) {
        elements.push(<h3 class="text-base font-semibold text-text-strong mt-3 mb-1">{line.slice(4)}</h3>)
        continue
      }
      if (line.startsWith('## ')) {
        elements.push(<h2 class="text-lg font-semibold text-text-strong mt-4 mb-2">{line.slice(3)}</h2>)
        continue
      }
      if (line.startsWith('# ')) {
        elements.push(<h1 class="text-xl font-bold text-text-strong mt-4 mb-2">{line.slice(2)}</h1>)
        continue
      }

      // List items
      if (line.match(/^[-*]\s/)) {
        elements.push(
          <div class="flex gap-2 ml-2">
            <span class="text-text-weaker">•</span>
            <span class="text-sm text-text-base">{formatInline(line.slice(2))}</span>
          </div>
        )
        continue
      }

      // Numbered list
      if (line.match(/^\d+\.\s/)) {
        const num = line.match(/^(\d+)\./)?.[1]
        elements.push(
          <div class="flex gap-2 ml-2">
            <span class="text-text-weaker">{num}.</span>
            <span class="text-sm text-text-base">{formatInline(line.replace(/^\d+\.\s/, ''))}</span>
          </div>
        )
        continue
      }

      // Empty line
      if (!line.trim()) {
        elements.push(<div class="h-2" />)
        continue
      }

      // Regular paragraph
      elements.push(<p class="text-sm text-text-base">{formatInline(line)}</p>)
    }

    return elements
  })

  return <div class="space-y-1">{rendered()}</div>
}

function formatInline(text: string): JSX.Element {
  // Bold
  const parts: (string | JSX.Element)[] = []
  let remaining = text

  // Process **bold** and `code`
  const regex = /(\*\*(.+?)\*\*)|(`(.+?)`)/g
  let lastIndex = 0
  let match

  while ((match = regex.exec(text)) !== null) {
    // Add text before match
    if (match.index > lastIndex) {
      parts.push(text.slice(lastIndex, match.index))
    }

    if (match[2]) {
      // Bold
      parts.push(<strong class="font-semibold">{match[2]}</strong>)
    } else if (match[4]) {
      // Inline code
      parts.push(
        <code class="px-1 py-0.5 text-xs bg-background-raised rounded font-mono text-accent-secondary">
          {match[4]}
        </code>
      )
    }

    lastIndex = match.index + match[0].length
  }

  // Add remaining text
  if (lastIndex < text.length) {
    parts.push(text.slice(lastIndex))
  }

  return <>{parts}</>
}

function CodePart(props: { content: string; language?: string }) {
  async function copyCode() {
    await navigator.clipboard.writeText(props.content)
  }

  return (
    <div class="relative group/code rounded-lg overflow-hidden border border-border-base">
      {/* Header */}
      <div class="flex items-center justify-between px-3 py-1.5 bg-background-raised border-b border-border-base">
        <span class="text-xs text-text-weaker font-mono">{props.language || "text"}</span>
        <button
          onClick={copyCode}
          class="px-2 py-0.5 text-xs text-text-weaker hover:text-text-base
                 opacity-0 group-hover/code:opacity-100 transition-opacity"
        >
          Copy
        </button>
      </div>

      {/* Code */}
      <pre class="p-3 overflow-x-auto bg-background-base">
        <code class="text-xs font-mono text-text-base whitespace-pre">{props.content}</code>
      </pre>
    </div>
  )
}

function ToolPart(props: { tool: ToolCall }) {
  return (
    <div class="rounded-lg border border-border-base overflow-hidden">
      {/* Tool header */}
      <div
        classList={{
          "flex items-center gap-2 px-3 py-2": true,
          "bg-yellow-500/10 border-yellow-500/20": props.tool.status === "running",
          "bg-green-500/10": props.tool.status === "done",
          "bg-red-500/10": props.tool.status === "error",
          "bg-background-raised": props.tool.status === "pending"
        }}
      >
        {/* Status icon */}
        <Show when={props.tool.status === "running"}>
          <svg class="w-4 h-4 text-yellow-500 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
        </Show>
        <Show when={props.tool.status === "done"}>
          <svg class="w-4 h-4 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
          </svg>
        </Show>
        <Show when={props.tool.status === "error"}>
          <svg class="w-4 h-4 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </Show>

        <span class="text-sm font-medium text-text-base">{props.tool.name}</span>
        <span class="text-xs text-text-weaker ml-auto">{props.tool.status}</span>
      </div>

      {/* Tool input */}
      <Show when={props.tool.input}>
        <div class="px-3 py-2 bg-background-base border-t border-border-base">
          <div class="text-xs text-text-weaker mb-1">Input</div>
          <pre class="text-xs font-mono text-text-base whitespace-pre-wrap overflow-x-auto">
            {props.tool.input}
          </pre>
        </div>
      </Show>

      {/* Tool output */}
      <Show when={props.tool.output}>
        <div class="px-3 py-2 bg-background-base border-t border-border-base">
          <div class="text-xs text-text-weaker mb-1">Output</div>
          <pre class="text-xs font-mono text-text-base whitespace-pre-wrap overflow-x-auto max-h-40">
            {props.tool.output}
          </pre>
        </div>
      </Show>
    </div>
  )
}

// Helper to parse raw message content into parts
export function parseMessageContent(content: string): MessagePart[] {
  const parts: MessagePart[] = []

  // Check for tool calls [Tool: name]
  const toolMatch = content.match(/\[Tool: ([^\]]+)\]/)
  if (toolMatch) {
    // Split around tool markers
    const segments = content.split(/\[Tool: [^\]]+\]|\[Tool done\]/)

    for (let i = 0; i < segments.length; i++) {
      const seg = segments[i].trim()
      if (seg) {
        parts.push({ type: "text", content: seg })
      }

      // Add tool part after each segment (except last)
      if (i < segments.length - 1) {
        const toolNames = content.match(/\[Tool: ([^\]]+)\]/g)
        if (toolNames && toolNames[i]) {
          const name = toolNames[i].replace('[Tool: ', '').replace(']', '')
          parts.push({
            type: "tool",
            tool: {
              name,
              input: "",
              status: "done"
            }
          })
        }
      }
    }
  } else {
    parts.push({ type: "text", content })
  }

  return parts
}
