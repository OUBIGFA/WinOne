// Copy only the source UI domains, merging into WinOne's single locale store.
import {i18n} from '../../../i18n';
import en from './en.json';
import zh from './zh-CN.json';
i18n.addResourceBundle('en-US','translation',en,true,false);
i18n.addResourceBundle('zh-CN','translation',zh,true,false);
