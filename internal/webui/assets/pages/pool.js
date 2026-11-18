import {renderList} from './topics.js';
// The pool deliberately uses the same server projection and row component as
// the Topics pool scope. Filtering never renumbers its stored queue positions.
export async function render(ctx) { return renderList(ctx,true); }
