
import {Controller} from 'cerebral';
import DevTools from 'cerebral/devtools';
import HttpProvider from '@cerebral/http';
import StorageProvider from '@cerebral/storage'

const controller = Controller({
    devtools: DevTools({ host: 'localhost:8787' }),
    providers: [
        StorageProvider({
            target: sessionStorage,
            sync: {},
            prefix: "callback-ui"
        }),
        HttpProvider({
            baseUrl: contextPath + '/api/v1'
        })
    ],
    state: {
    },
    signals: {
        appMounted: []
    }
});

export default controller;
