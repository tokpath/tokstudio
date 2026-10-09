import {PublicModelInstructions} from "@/components/public-model-instructions";
export default async function IntegrationsPage({searchParams}: {searchParams: Promise<{model?: string}>}) {
 const {model} = await searchParams;
 return <PublicModelInstructions modelID={model} tab="agent"/>;
}
