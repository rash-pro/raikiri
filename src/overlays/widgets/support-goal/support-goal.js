import { connectEvents } from "/shared/ws-client.js";
import { applyWidgetAppearance } from "/shared/widget-runtime.js";

const root = document.getElementById('goal');
const title = document.getElementById('goal-title');
const amount = document.getElementById('goal-amount');
const fill = document.getElementById('goal-fill');

const params = new URLSearchParams(window.location.search);
const prefix = params.get('prefix') || '$';

function formatMoney(value, currency) {
    const rounded = Number(value || 0).toLocaleString(undefined, { maximumFractionDigits: 2 });
    return currency ? `${currency} ${rounded}` : `${prefix}${rounded}`;
}

function render(state) {
    const goal = state?.supportGoal;
    if (!goal || goal.enabled === false) {
        root.hidden = true;
        return;
    }
    applyWidgetAppearance(goal.appearance || {});
    root.hidden = false;
    const target = Number(goal.targetAmount || 0);
    const current = Number(goal.currentAmount || 0);
    const pct = target > 0 ? Math.min(100, Math.max(0, (current / target) * 100)) : 0;
    title.textContent = goal.title || 'Support Goal';
    amount.textContent = `${formatMoney(current, goal.currency)} / ${formatMoney(target, goal.currency)}`;
    fill.style.width = `${pct}%`;
}

fetch('/api/widgets/state')
    .then(res => res.json())
    .then(render)
    .catch(console.error);

connectEvents('/ws/widgets', {
    state: render,
    connect: () => console.log('Connected to Support Goal widget')
});
