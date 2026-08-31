'use client';

import { useId } from 'react';

interface FieldProps {
    label: string;
    type?: string;
    value: string;
    onChange: (value: string) => void;
    required?: boolean;
    autoComplete?: string;
    /** Field-level error. Rendered next to the input and linked with
     *  aria-describedby, so a screen reader reaches it from the field rather
     *  than only from a summary at the top of the form. */
    error?: string;
    hint?: string;
    disabled?: boolean;
}

export function Field({
    label, type = 'text', value, onChange,
    required, autoComplete, error, hint, disabled,
}: FieldProps) {
    // useId keeps label/input/description wired together without hand-managed
    // ids, and stays stable across server and client renders.
    const id = useId();
    const hintId = `${id}-hint`;
    const errorId = `${id}-error`;

    return (
        <div className="flex flex-col gap-1.5">
            {/* A visible label, not a placeholder. A placeholder disappears the
                moment typing starts, which is exactly when it is needed. */}
            <label htmlFor={id} className="text-sm font-medium text-text">
                {label}
                {required && <span className="ml-1 text-danger" aria-hidden="true">*</span>}
            </label>

            <input
                id={id}
                type={type}
                value={value}
                onChange={(e) => onChange(e.target.value)}
                required={required}
                autoComplete={autoComplete}
                disabled={disabled}
                aria-invalid={error ? true : undefined}
                aria-describedby={error ? errorId : hint ? hintId : undefined}
                className={[
                    'h-11 w-full rounded-md border bg-surface px-3 text-[15px] text-text',
                    'placeholder:text-text-muted',
                    'transition-[border-color,box-shadow] duration-[120ms]',
                    'disabled:cursor-not-allowed disabled:opacity-60',
                    error
                        ? 'border-danger focus:border-danger'
                        : 'border-border-strong focus:border-accent',
                ].join(' ')}
            />

            {hint && !error && (
                <p id={hintId} className="text-[13px] text-text-muted">{hint}</p>
            )}

            {error && (
                // role="alert" so the message is announced when it appears,
                // without moving focus away from the field.
                <p id={errorId} role="alert" className="text-[13px] font-medium text-danger">
                    {error}
                </p>
            )}
        </div>
    );
}
