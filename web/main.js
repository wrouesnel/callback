import moment from 'moment'
moment.locale(window.navigator.userLanguage || window.navigator.language);

import React from 'react';
import {Container} from 'cerebral/react';
import ReactDOM from 'react-dom';

import controller from 'controller.js';

import 'bootstrap/dist/css/bootstrap.css';
import 'bootstrap/dist/css/bootstrap-theme.css';

import App from 'components/App.js';

ReactDOM.render(
    <Container controller={controller}>
        <App/>
    </Container>
    , document.querySelector('#app'));
