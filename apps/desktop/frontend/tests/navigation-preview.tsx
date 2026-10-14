import {StrictMode,useState} from 'react';
import {createRoot} from 'react-dom/client';
import {usePageNavigation,usePageActivity} from '../src/ui/page-navigation';
import {AppShell} from '../src/components/app-shell';
import {useSettingsNavigation} from '../src/ui/settings-navigation';
import {Select,SelectTrigger,SelectValue,SelectContent,SelectItem} from '../src/components/ui/select';
import '../src/styles/shadcn.css';
import '../src/style.css';
let sequence=0;let notify=()=>{};
const port={read:async()=>sequence,subscribe:(listener:()=>void)=>{notify=listener;return()=>{notify=()=>{};};}};
// Simulate the native reopen request without operating a desktop window.
Object.assign(window,{reopenSettings:()=>{sequence++;notify();}});
function Preview(){
 const [page,setPage]=useState('edit');const [text,setText]=useState('');
 const nav=usePageNavigation(page,setPage);usePageActivity(nav.onActivity,text!==''&&page==='edit',false);
 useSettingsNavigation(port,nav.requestPage,console.error);
 return <AppShell page={page} pages={[]} logo="" onNavigate={nav.requestPage}><input aria-label="draft" value={text} onChange={e=>setText(e.target.value)}/><Select defaultValue="one" items={[{value:'one',label:'One'},{value:'two',label:'Two'}]}><SelectTrigger aria-label="mode"><SelectValue/></SelectTrigger><SelectContent><SelectItem value="one">One</SelectItem><SelectItem value="two">Two</SelectItem></SelectContent></Select><button onClick={()=>nav.requestPage('next')}>leave</button><p data-testid="page">{page}</p>{nav.dialog}</AppShell>;
}
createRoot(document.getElementById('root')!).render(<StrictMode><Preview/></StrictMode>);
