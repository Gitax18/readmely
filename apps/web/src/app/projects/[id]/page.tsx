import type { Metadata } from "next";
import { OverviewTab } from "@/components/project/overview-tab";

export const metadata: Metadata = { title: "Project · Readmely" };

export default function ProjectOverviewPage() {
  return <OverviewTab />;
}
