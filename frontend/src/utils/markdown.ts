import DOMPurify from "dompurify";
import { Marked } from "marked";
import markedKatex from "marked-katex-extension";

const markdown = new Marked(
  markedKatex({ output: "mathml", throwOnError: false, trust: false })
);

export async function renderMarkdown(source: string): Promise<string> {
  if (!DOMPurify.isSupported) {
    throw new Error("This browser does not support safe Markdown previews");
  }
  return DOMPurify.sanitize(await markdown.parse(source));
}
