import type { Metadata } from "next";
import { ReadmeTab } from "@/components/project/tabs";

export const metadata: Metadata = { title: "README · Readmely" };

export default function ProjectReadmePage() {
  return <ReadmeTab />;
}
