import type { Metadata } from "next";
import { DiagramTab } from "@/components/project/tabs";

export const metadata: Metadata = { title: "Diagram · Readmely" };

export default function ProjectDiagramPage() {
  return <DiagramTab />;
}
