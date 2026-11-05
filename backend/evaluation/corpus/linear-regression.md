# Introduction to Linear Regression

## What linear regression does

Linear regression models the relationship between a numeric outcome and one or more input variables by fitting a straight line. In simple linear regression there is a single input, and the model is written y = b0 + b1x, where b0 is the intercept and b1 is the slope. The intercept is the predicted value of y when x is zero, and the slope is the expected change in y for a one-unit increase in x.

## Least squares

The most common way to fit the line is the method of ordinary least squares. A residual is the difference between an observed value and the value the line predicts. Ordinary least squares chooses the slope and intercept that make the sum of the squared residuals as small as possible. Squaring the residuals stops positive and negative errors from cancelling out and gives large errors more weight.

## Measuring fit

The coefficient of determination, written R squared, measures the share of the variation in y that the model explains. R squared ranges from 0 to 1, where 1 means the line explains all of the variation. A high R squared does not prove that x causes y, and adding more input variables never lowers R squared, which is why adjusted R squared is used to compare models with different numbers of inputs.

## Assumptions

Linear regression relies on several assumptions. The relationship between the inputs and the outcome should be linear. The residuals should be independent of each other, should have constant variance across all values of x, a property called homoscedasticity, and should be roughly normally distributed. When the spread of the residuals grows as x grows, the data show heteroscedasticity, which makes the usual standard errors unreliable.

## Multiple regression and multicollinearity

Multiple linear regression uses two or more input variables, and each coefficient is interpreted as the effect of its variable while the other inputs are held constant. Multicollinearity occurs when input variables are strongly correlated with each other; it makes the individual coefficients unstable and hard to interpret, even if the overall predictions remain good.

## Overfitting and evaluation

A model overfits when it learns noise in the training data and then performs poorly on new data. To check for this, the data are split into a training set used to fit the model and a test set used to evaluate it. A common error measure is the root mean squared error, or RMSE, which is the square root of the average squared prediction error and is expressed in the same units as y.
