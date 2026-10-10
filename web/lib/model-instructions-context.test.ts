import {describe, expect, it} from "vitest";
import {instructionsQuery} from "./model-instructions-context";

describe("model instructions entry context",()=>{
  it("keeps the selected model, Key, tool, language and invitation when choosing a model",()=>{
    const params=instructionsQuery({key_id:"key-a",tool:"aider",protocol:"/v1/chat/completions",language:"node_sdk",promo:"invite",return_to:"/models?q=gpt"},"vendor/new","agent");
    expect(Object.fromEntries(params)).toEqual({model:"vendor/new",tab:"agent",key_id:"key-a",tool:"aider",protocol:"/v1/chat/completions",language:"node_sdk",promo:"invite",return_to:"/models?q=gpt"});
  });
  it.each(["https://evil.example/","//evil.example/","/\\evil.example/","/%2f%2fevil.example/"])("drops an invalid return target %s",return_to=>{
    expect(instructionsQuery({return_to},"model","protocol").has("return_to")).toBe(false);
  });
});
