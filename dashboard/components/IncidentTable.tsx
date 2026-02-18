import { Incident } from '../types/incident';
import { format } from 'date-fns';
import { AlertCircle, CheckCircle, Terminal, ChevronDown, ChevronUp, Eye } from 'lucide-react';
import { Fragment, useState } from 'react';

interface IncidentTableProps {
    incidents: Incident[];
}

export default function IncidentTable({ incidents }: IncidentTableProps) {
    const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());

    const toggleRow = (id: number) => {
        setExpandedRows(prev => {
            const next = new Set(prev);
            if (next.has(id)) {
                next.delete(id);
            } else {
                next.add(id);
            }
            return next;
        });
    };

    const hasPattern = (incident: Incident) => {
        return (incident.exit_reason === 'LOOP_DETECTED' || incident.exit_reason === 'WATCHDOG_ALERT')
            && incident.pattern && incident.pattern !== 'N/A';
    };

    return (
        <div className="overflow-hidden rounded-xl border border-gray-800 bg-obsidian-800 shadow-lg">
            <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-800">
                    <thead className="bg-gray-900/50">
                        <tr>
                            <th className="px-6 py-4 text-left text-xs font-semibold text-gray-400 uppercase tracking-wider">Status</th>
                            <th className="px-6 py-4 text-left text-xs font-semibold text-gray-400 uppercase tracking-wider">Timestamp</th>
                            <th className="px-6 py-4 text-left text-xs font-semibold text-gray-400 uppercase tracking-wider">Command</th>
                            <th className="px-6 py-4 text-right text-xs font-semibold text-gray-400 uppercase tracking-wider">CPU</th>
                            <th className="px-6 py-4 text-left text-xs font-semibold text-gray-400 uppercase tracking-wider">Pattern</th>
                            <th className="px-6 py-4 text-left text-xs font-semibold text-gray-400 uppercase tracking-wider">ID</th>
                        </tr>
                    </thead>
                    <tbody className="divide-y divide-gray-800 bg-obsidian-800">
                        {incidents.map((incident) => (
                            <Fragment key={incident.id}>
                                <tr key={incident.id} className="hover:bg-gray-800/50 transition-colors">
                                    <td className="px-6 py-4 whitespace-nowrap">
                                        <span
                                            className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium border ${incident.exit_reason === 'LOOP_DETECTED'
                                                ? 'bg-red-500/10 text-red-400 border-red-500/20'
                                                : incident.exit_reason === 'WATCHDOG_ALERT'
                                                    ? 'bg-amber-500/10 text-amber-400 border-amber-500/20'
                                                    : incident.exit_reason === 'SUCCESS'
                                                        ? 'bg-green-500/10 text-green-400 border-green-500/20'
                                                        : 'bg-gray-500/10 text-gray-400 border-gray-500/20'
                                                }`}
                                        >
                                            {incident.exit_reason === 'LOOP_DETECTED' ? (
                                                <AlertCircle size={12} />
                                            ) : incident.exit_reason === 'WATCHDOG_ALERT' ? (
                                                <Eye size={12} />
                                            ) : incident.exit_reason === 'SUCCESS' ? (
                                                <CheckCircle size={12} />
                                            ) : (
                                                <Terminal size={12} />
                                            )}
                                            {incident.exit_reason.replace('_', ' ')}
                                        </span>
                                    </td>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-400">
                                        {format(new Date(incident.timestamp), 'MMM d, HH:mm:ss')}
                                    </td>
                                    <td className="px-6 py-4 whitespace-nowrap">
                                        <div className="flex items-center">
                                            <span className="font-mono text-sm text-gray-200 bg-gray-900 px-2 py-1 rounded border border-gray-700">
                                                $ {incident.command}
                                            </span>
                                        </div>
                                    </td>
                                    <td className="px-6 py-4 whitespace-nowrap text-right text-sm font-mono text-gray-300">
                                        {incident.max_cpu.toFixed(1)}%
                                    </td>
                                    <td className="px-6 py-4 whitespace-nowrap">
                                        {hasPattern(incident) ? (
                                            <button
                                                onClick={() => toggleRow(incident.id)}
                                                className={`inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium border transition-all duration-200 cursor-pointer ${expandedRows.has(incident.id)
                                                        ? 'bg-accent-500/20 text-accent-300 border-accent-500/40 shadow-sm shadow-accent-500/10'
                                                        : 'bg-accent-500/10 text-accent-400 border-accent-500/20 hover:bg-accent-500/20'
                                                    }`}
                                            >
                                                {expandedRows.has(incident.id) ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
                                                {expandedRows.has(incident.id) ? 'Hide' : 'Show'}
                                            </button>
                                        ) : (
                                            <span className="text-xs text-gray-600">—</span>
                                        )}
                                    </td>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-600 font-mono">
                                        #{incident.id}
                                    </td>
                                </tr>
                                {/* Pattern Highlight Expandable Row */}
                                {hasPattern(incident) && expandedRows.has(incident.id) && (
                                    <tr key={`pattern-${incident.id}`}>
                                        <td colSpan={6} className="px-0 py-0">
                                            <div className={`mx-4 my-3 rounded-xl border p-5 transition-all duration-300 animate-in ${incident.exit_reason === 'LOOP_DETECTED'
                                                    ? 'border-red-500/30 bg-gradient-to-r from-red-950/40 to-red-900/10'
                                                    : 'border-amber-500/30 bg-gradient-to-r from-amber-950/40 to-amber-900/10'
                                                }`}>
                                                <div className="flex items-start gap-4">
                                                    <div className={`rounded-lg p-2 ${incident.exit_reason === 'LOOP_DETECTED'
                                                            ? 'bg-red-500/15 text-red-400'
                                                            : 'bg-amber-500/15 text-amber-400'
                                                        }`}>
                                                        <AlertCircle size={18} />
                                                    </div>
                                                    <div className="flex-1 min-w-0">
                                                        <div className="flex items-center justify-between mb-3">
                                                            <h4 className={`text-sm font-semibold ${incident.exit_reason === 'LOOP_DETECTED'
                                                                    ? 'text-red-300'
                                                                    : 'text-amber-300'
                                                                }`}>
                                                                Fuzzy Pattern Match — Levenshtein ≥90%
                                                            </h4>
                                                            <span className="text-xs text-gray-500 font-mono">
                                                                Model: {incident.model_name || 'unknown'}
                                                            </span>
                                                        </div>
                                                        <p className="text-xs text-gray-400 mb-3">
                                                            This is the <strong>normalized</strong> log pattern that triggered the alarm. Timestamps, hex addresses, and numbers are replaced with placeholders to detect semantic repetition.
                                                        </p>
                                                        <div className="relative">
                                                            <code className={`block text-sm font-mono p-4 rounded-lg border whitespace-pre-wrap break-all ${incident.exit_reason === 'LOOP_DETECTED'
                                                                    ? 'bg-black/40 border-red-500/20 text-red-200'
                                                                    : 'bg-black/40 border-amber-500/20 text-amber-200'
                                                                }`}>
                                                                {incident.pattern}
                                                            </code>
                                                            <div className={`absolute top-2 right-2 text-[10px] font-mono px-1.5 py-0.5 rounded ${incident.exit_reason === 'LOOP_DETECTED'
                                                                    ? 'bg-red-500/20 text-red-400'
                                                                    : 'bg-amber-500/20 text-amber-400'
                                                                }`}>
                                                                NORMALIZED
                                                            </div>
                                                        </div>
                                                        <div className="mt-3 flex items-center gap-4 text-xs text-gray-500">
                                                            <span>Savings: <strong className="text-green-400">${incident.token_savings_estimate.toFixed(4)}</strong></span>
                                                            <span>Peak CPU: <strong className="text-gray-300">{incident.max_cpu.toFixed(1)}%</strong></span>
                                                        </div>
                                                        {(incident.reason || incident.confidence_score > 0) && (
                                                            <div className="mt-4 rounded-lg border border-gray-700 bg-gray-900/50 p-3">
                                                                <h5 className="mb-2 text-xs font-semibold uppercase tracking-wider text-gray-400">
                                                                    Reason For Action
                                                                </h5>
                                                                <p className="mb-3 text-xs text-gray-300">
                                                                    {incident.reason || "No explicit reason recorded."}
                                                                </p>
                                                                <div className="grid grid-cols-3 gap-3 text-xs">
                                                                    <div>
                                                                        <p className="text-gray-500">CPU Score</p>
                                                                        <p className="font-mono text-red-300">{incident.cpu_score?.toFixed(1) || "0.0"}</p>
                                                                    </div>
                                                                    <div>
                                                                        <p className="text-gray-500">Entropy Score</p>
                                                                        <p className="font-mono text-amber-300">{incident.entropy_score?.toFixed(1) || "0.0"}</p>
                                                                    </div>
                                                                    <div>
                                                                        <p className="text-gray-500">Confidence</p>
                                                                        <p className="font-mono text-accent-300">{incident.confidence_score?.toFixed(1) || "0.0"}</p>
                                                                    </div>
                                                                </div>
                                                                <p className="mt-2 text-[11px] text-gray-500">
                                                                    Confidence combines CPU pressure and repetition entropy to explain why action was taken.
                                                                </p>
                                                            </div>
                                                        )}
                                                    </div>
                                                </div>
                                            </div>
                                        </td>
                                    </tr>
                                )}
                            </Fragment>
                        ))}
                        {incidents.length === 0 && (
                            <tr>
                                <td colSpan={6} className="px-6 py-12 text-center text-gray-500">
                                    No incidents recorded yet.
                                </td>
                            </tr>
                        )}
                    </tbody>
                </table>
            </div>
        </div>
    );
}
