import type { Metadata } from "next";
import { ContextPackTab } from "@/components/project/tabs";

export const metadata: Metadata = { title: "Context pack · Readmely" };

export default function ProjectContextPackPage() {
  return <ContextPackTab />;
}
