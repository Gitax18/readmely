import { Skeleton } from "@/components/ui/skeleton";

/** The fallback while any page loads: a quiet placeholder shaped like a page. */
export default function Loading() {
  return (
    <div role="status" aria-label="Loading" className="mx-auto w-full max-w-[860px] px-4 py-10">
      <Skeleton className="h-8 w-48" />
      <Skeleton className="mt-3 h-4 w-80 max-w-full" />
      <Skeleton className="mt-8 h-40 w-full rounded-card" />
    </div>
  );
}
