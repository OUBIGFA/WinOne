// Exercise the real source worker with optional execution libraries unavailable.
// Browsing an empty OneBot home must not evaluate any of these modules.
import {plugin} from 'bun';
plugin({name:'onebot-startup-import-guard',setup(builder){
  builder.onLoad({filter:/[\\/]node_modules[\\/](?:@earendil-works[\\/]pi-coding-agent|@larksuiteoapi|discord\.js|@wecom|dingtalk-stream)[\\/]/},({path})=>{
    throw Error('Optional execution library loaded during OneBot startup: '+path);
  });
}});
