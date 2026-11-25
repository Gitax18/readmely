import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MarkdownView, headingAnchor } from "./markdown";

const README = [
  "# Demo",
  "",
  "![Go](https://img.shields.io/badge/Go-00ADD8?style=for-the-badge) ![tracker](https://evil.example/pixel.png)",
  "",
  "**Contents:** [Tech stack](#tech-stack) · [Docs](https://example.com/docs)",
  "",
  "## Tech stack",
  "",
  "```mermaid",
  "flowchart LR",
  '  a["Web"] --> b["API"]',
  "```",
  "",
  "```bash",
  "make dev",
  "```",
].join("\n");

describe("MarkdownView", () => {
  it("never renders an image, showing badges as chips and anything else as its alt text", () => {
    const { container } = render(<MarkdownView source={README} />);
    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText("Go").tagName).toBe("SPAN");
    expect(screen.getByText("[tracker]")).toBeInTheDocument();
  });

  it("links a contents entry to its heading in the preview and opens other links in a new tab", () => {
    render(<MarkdownView source={README} />);
    const heading = screen.getByRole("heading", { level: 2, name: "Tech stack" });
    expect(heading.id).toBe("readme-tech-stack");
    expect(screen.getByRole("link", { name: "Tech stack" })).toHaveAttribute(
      "href",
      "#readme-tech-stack",
    );
    expect(screen.getByRole("link", { name: "Tech stack" })).not.toHaveAttribute("target");
    expect(screen.getByRole("link", { name: "Docs" })).toHaveAttribute("target", "_blank");
  });

  it("notes that GitHub draws a Mermaid block, and leaves other code alone", () => {
    const { container } = render(<MarkdownView source={README} />);
    expect(screen.getAllByText("Mermaid diagram. GitHub draws it from this code.")).toHaveLength(1);
    expect(container.querySelectorAll("figure pre")).toHaveLength(1);
    expect(container.querySelectorAll("pre")).toHaveLength(2);
  });
});

describe("headingAnchor", () => {
  it("matches GitHub's anchors", () => {
    expect(headingAnchor("Getting started")).toBe("readme-getting-started");
    expect(headingAnchor("FAQ & Help!")).toBe("readme-faq--help");
  });
});
