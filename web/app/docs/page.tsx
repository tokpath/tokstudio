import {PublicModelInstructions} from "@/components/public-model-instructions";
import type {InstructionsQuery} from "@/lib/model-instructions-context";
export default async function DocsPage({searchParams}: {searchParams: Promise<InstructionsQuery>}) {
 const query = await searchParams;
 return <PublicModelInstructions modelID={query.model} tab={query.tab === "agent" ? "agent" : "protocol"} query={query}/>;
}
