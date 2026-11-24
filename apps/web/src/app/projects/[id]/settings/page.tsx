import type { Metadata } from "next";
import { SettingsTab } from "@/components/project/settings-tab";

export const metadata: Metadata = { title: "Project settings · Readmely" };

export default function ProjectSettingsPage() {
  return <SettingsTab />;
}
