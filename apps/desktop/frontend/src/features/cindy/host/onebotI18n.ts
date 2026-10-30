// Use WinOne's already initialized i18next instance; never initialize a second
// language store from Cindy's app bootstrap.
import i18next from 'i18next';
export default i18next;
export {i18next as i18n};
