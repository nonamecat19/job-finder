import { Tile } from '../../components/layout';

interface SalaryEstimateCardProps {
  estimateRaw?: string | null;
  estimateMin?: number | null;
  estimateMax?: number | null;
  estimateCurrency?: string | null;
  isEstimated: boolean;
}

function fmtCurrency(n: number): string {
  return n.toLocaleString('en-US');
}

export default function SalaryEstimateCard({
  estimateRaw, estimateMin, estimateMax, estimateCurrency, isEstimated,
}: SalaryEstimateCardProps) {
  if (!isEstimated && !estimateRaw) return null;

  const range = estimateMin != null && estimateMax != null
    ? `${estimateCurrency ?? '$'}${fmtCurrency(estimateMin)} – ${estimateCurrency ?? '$'}${fmtCurrency(estimateMax)}`
    : estimateRaw ?? '';

  return (
    <Tile span="standard" title="Salary Estimate">
      <div className="flex flex-col gap-1">
        <span className="text-lg font-semibold">{range}</span>
        <span className="text-xs text-faint">Estimated from similar positions on Djinni</span>
      </div>
    </Tile>
  );
}
