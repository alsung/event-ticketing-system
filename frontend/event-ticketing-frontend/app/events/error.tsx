'use client';

import { Button } from '@/components/ui/Button';

/** Error boundary for the events route. `reset` re-renders the segment, which
 *  re-runs the server fetch -- so the recovery path is a real retry rather than
 *  a suggestion to reload the page. */
export default function EventsError({ reset }: { error: Error; reset: () => void }) {
    return (
        <div className="flex flex-col items-start gap-4 rounded-lg border border-border bg-surface p-6">
            <div className="flex flex-col gap-1.5">
                <h2>Could not load events</h2>
                <p className="max-w-md text-[15px] text-text-secondary">
                    The events service did not respond. It may still be starting up — if you
                    are running this locally, check that <code className="rounded bg-surface-sunken px-1.5 py-0.5 text-[13px]">make up</code> has finished.
                </p>
            </div>
            <Button variant="secondary" onClick={reset}>Try again</Button>
        </div>
    );
}
