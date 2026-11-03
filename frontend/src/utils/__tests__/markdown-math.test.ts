// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { renderMarkdown } from "@/utils/markdown";

describe("Markdown with KaTeX 0.19", () => {
  it("keeps inline and display MathML after sanitization", async () => {
    const html = await renderMarkdown("Inline $x^2$\n\n$$\n\\frac{1}{2}\n$$");
    const document = new DOMParser().parseFromString(html, "text/html");
    expect(document.querySelectorAll("math")).toHaveLength(2);
    expect(document.querySelector("msup")?.textContent).toContain("x2");
    expect(document.querySelector("mfrac")?.textContent).toContain("12");
  });

  it("removes scripts, event handlers, and unsafe link targets", async () => {
    const html = await renderMarkdown(
      '<script>alert(1)</script><img src=x onerror="alert(1)">' +
        "\n\n[bad](javascript:alert(1))\n\n$\\href{javascript:alert(1)}{x}$"
    );
    const document = new DOMParser().parseFromString(html, "text/html");
    expect(document.querySelector("script,[onerror]")).toBeNull();
    expect(document.querySelector('[href^="javascript:"]')).toBeNull();
  });

  it("handles invalid math without breaking the editor preview", async () => {
    expect(await renderMarkdown("$\\unknowncommand{x}$")).toContain(
      "unknowncommand"
    );
    expect(await renderMarkdown("**still works**")).toContain(
      "<strong>still works</strong>"
    );
  });
});
